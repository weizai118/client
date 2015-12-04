// Copyright 2015 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package engine

import (
	"github.com/keybase/client/go/libkb"
	keybase1 "github.com/keybase/client/go/protocol"
	"sync"
	"time"
)

var locktab libkb.LockTable

//
// TODOs:
//   - mocked out proof checkers for some sort of testing ability
//     - this probably means something other than NewProofChecker() in ID table,
//       and replacing it with a Mock instead.
//   - think harder about what we're caching in failure cases; right now we're only
//     caching full successes.
//

// Identify2 is the Identify engine used in KBFS and as a subroutine
// of command-line crypto.
type Identify2 struct {
	libkb.Contextified

	arg        *keybase1.Identify2Arg
	trackToken keybase1.TrackToken
	cachedRes  *keybase1.UserPlusKeys

	me   *libkb.User
	them *libkb.User

	themAssertion   libkb.AssertionExpression
	remoteAssertion libkb.AssertionAnd
	localAssertion  libkb.AssertionAnd

	state        libkb.IdentifyState
	useTracking  bool
	identifyKeys []keybase1.IdentifyKey

	ranAtTime time.Time
}

var _ (Engine) = (*Identify2)(nil)

// Name is the unique engine name.
func (e *Identify2) Name() string {
	return "Identify2"
}

func NewIdentify2(g *libkb.GlobalContext, arg *keybase1.Identify2Arg) *Identify2 {
	return &Identify2{
		Contextified: libkb.NewContextified(g),
		arg:          arg,
	}
}

// GetPrereqs returns the engine prereqs.
func (e *Identify2) Prereqs() Prereqs {
	return Prereqs{}
}

// Run then engine
func (e *Identify2) Run(ctx *Context) (err error) {

	e.G().Log.Debug("+ Identify2.runSingle(UID=%v, Assertion=%s)", e.arg.Uid, e.arg.UserAssertion)

	e.G().Log.Debug("+ acquire singleflight lock")
	lock := locktab.LockOnName(e.arg.Uid.String())
	e.G().Log.Debug("- acquired")

	defer func() {
		if lock != nil {
			lock.Unlock()
		}
		e.G().Log.Debug("- Identify2.Run -> %v", err)
	}()

	if e.loadAssertion(); err != nil {
		return err
	}

	if !e.useAnyAssertions() && e.checkFastCacheHit() {
		e.G().Log.Debug("| was a self load")
		return nil
	}

	e.G().Log.Debug("| Identify2.loadUsers")
	if err = e.loadUsers(ctx); err != nil {
		return err
	}

	if err = e.checkLocalAssertions(); err != nil {
		return err
	}

	if e.isSelfLoad() {
		e.G().Log.Debug("| was a self load")
		return nil
	}

	if !e.useRemoteAssertions() && e.checkSlowCacheHit() {
		e.G().Log.Debug("| hit slow cache, first check")
		return nil
	}

	e.G().Log.Debug("| Identify2.createIdentifyState")
	if err = e.createIdentifyState(); err != nil {
		return err
	}

	if err = e.runIdentifyPrecomputation(); err != nil {
		return err
	}

	// First we check that all remote assertions as present for the user,
	// whether or not the remote check actually suceeds (hnece the
	// ProofState_NONE check).
	if err = e.checkRemoteAssertions([]keybase1.ProofState{keybase1.ProofState_NONE}); err != nil {
		e.G().Log.Debug("| Early fail due to missing remote assertions")
		return err
	}

	if e.useRemoteAssertions() && e.checkSlowCacheHit() {
		e.G().Log.Debug("| hit slow cache, second check")
		return nil
	}

	if err = e.startIdentifyUI(ctx); err != nil {
		return err
	}

	l2 := lock
	lock = nil
	if err = e.finishIdentify(ctx, l2); err != nil {
		return err
	}

	e.ranAtTime = time.Now()
	return nil
}

func (e *Identify2) runIDTableIdentify(ctx *Context, lock *libkb.NamedLock) (err error) {
	e.them.IDTable().Identify(e.state, false /* ForceRemoteCheck */, ctx.IdentifyUI, nil)
	e.insertTrackToken(ctx)
	err = e.checkRemoteAssertions([]keybase1.ProofState{keybase1.ProofState_OK})
	if err == nil {
		e.maybeCacheResult()
	}
	lock.Unlock()
	return err
}

func (e *Identify2) maybeCacheResult() {
	if e.state.Result().IsOK() {
		v := e.toUserPlusKeys()
		e.G().UserCache.Insert(&v)
	}
}

func (e *Identify2) insertTrackToken(ctx *Context) (err error) {
	e.G().Log.Debug("+ insertTrackToken")
	defer func() {
		e.G().Log.Debug("- insertTrackToken -> %v", err)
	}()
	var ict libkb.IdentifyCacheToken
	ict, err = e.G().IdentifyCache.Insert(e.state.Result())
	if err != nil {
		return err
	}
	if err = ctx.IdentifyUI.ReportTrackToken(ict); err != nil {
		return err
	}
	return nil
}

type checkCompletedListener struct {
	sync.Mutex
	err       error
	ch        chan error
	needed    libkb.AssertionAnd
	received  libkb.ProofSet
	completed bool
	responded bool
}

func newCheckCompletedListener(ch chan error, proofs libkb.AssertionAnd) *checkCompletedListener {
	ret := &checkCompletedListener{
		ch:     ch,
		needed: proofs,
	}
	return ret
}

func (c *checkCompletedListener) CheckCompleted(lcr *libkb.LinkCheckResult) {
	c.Lock()
	defer c.Unlock()
	libkb.AddToProofSetNoChecks(lcr.GetLink(), &c.received)

	if err := lcr.GetError(); err != nil {
		c.err = err
	}

	// note(maxtaco): this is a little ugly in that it's O(n^2) where n is the number
	// of identities in the assertion. But I can't imagine n > 3, so this is fine
	// for now.
	c.completed = c.needed.MatchSet(c.received)

	if c.err != nil || c.completed {
		c.respond()
	}
}

func (c *checkCompletedListener) Done() {
	c.Lock()
	defer c.Unlock()
	c.respond()
}

func (c *checkCompletedListener) respond() {
	if c.ch != nil {
		if !c.completed && c.err == nil {
			c.err = libkb.IdentifyDidNotCompleteError{}
		}
		c.ch <- c.err
		c.ch = nil
	}
}

func (e *Identify2) partiallyAsyncIdentify(ctx *Context, ch chan error, lock *libkb.NamedLock) {
	ccl := newCheckCompletedListener(ch, e.remoteAssertion)
	e.them.IDTable().Identify(e.state, false /* ForceRemoteCheck */, ctx.IdentifyUI, ccl)
	e.insertTrackToken(ctx)
	e.maybeCacheResult()
	lock.Unlock()
}

func (e *Identify2) checkLocalAssertions() error {
	if !e.localAssertion.MatchSet(*e.them.BaseProofSet()) {
		return libkb.UnmetAssertionError{User: e.them.GetName(), Remote: false}
	}
	return nil
}

func (e *Identify2) checkRemoteAssertions(okStates []keybase1.ProofState) error {
	var ps libkb.ProofSet
	e.state.Result().AddProofsToSet(&ps, okStates)
	if !e.remoteAssertion.MatchSet(ps) {
		return libkb.UnmetAssertionError{User: e.them.GetName(), Remote: true}
	}
	return nil
}

func (e *Identify2) finishIdentify(ctx *Context, lock *libkb.NamedLock) (err error) {

	e.G().Log.Debug("+ finishIdentify")
	defer func() {
		e.G().Log.Debug("- finishIdentify -> %v", err)
	}()

	ctx.IdentifyUI.LaunchNetworkChecks(e.state.ExportToUncheckedIdentity(), e.them.Export())

	switch {
	case e.useTracking:
		e.G().Log.Debug("| Case 1: Using Tracking")
		e.runIDTableIdentify(ctx, lock)
	case !e.useRemoteAssertions():
		e.G().Log.Debug("| Case 2: No tracking, without remote assertions")
		go e.runIDTableIdentify(ctx, lock)
	default:
		e.G().Log.Debug("| Case 3: No tracking, with remote assertions")
		ch := make(chan error)
		go e.partiallyAsyncIdentify(ctx, ch, lock)
		err = <-ch
	}

	return err
}

func (e *Identify2) loadAssertion() (err error) {
	e.themAssertion, err = libkb.AssertionParseAndOnly(e.arg.UserAssertion)
	if err == nil {
		e.remoteAssertion, e.localAssertion = libkb.CollectAssertions(e.themAssertion)
	}
	return err
}

func (e *Identify2) useAnyAssertions() bool {
	return e.useLocalAssertions() || e.useRemoteAssertions()
}

func (e *Identify2) useLocalAssertions() bool {
	return e.localAssertion.Len() > 0
}
func (e *Identify2) useRemoteAssertions() bool {
	return e.remoteAssertion.Len() > 0
}

func (e *Identify2) getIdentifyTime() keybase1.Time {
	return keybase1.Time(e.ranAtTime.Unix())
}

func (e *Identify2) runIdentifyPrecomputation() (err error) {
	f := func(k keybase1.IdentifyKey) {
		e.identifyKeys = append(e.identifyKeys, k)
	}
	e.state.Precompute(f)
	return nil
}

func (e *Identify2) startIdentifyUI(ctx *Context) (err error) {
	e.G().Log.Debug("+ Identify(%s)", e.them.GetName())
	ctx.IdentifyUI.Start(e.them.GetName())
	for _, k := range e.identifyKeys {
		ctx.IdentifyUI.DisplayKey(k)
	}
	ctx.IdentifyUI.ReportLastTrack(libkb.ExportTrackSummary(e.state.TrackLookup(), e.them.GetName()))
	return nil
}

func (e *Identify2) createIdentifyState() (err error) {
	e.state = libkb.NewIdentifyState(nil, e.them)
	if e.me == nil {
		return nil
	}
	tlink, err := e.me.TrackChainLinkFor(e.them.GetName(), e.them.GetUID())
	if err != nil {
		return nil
	}
	if tlink != nil {
		e.useTracking = true
		e.state.SetTrackLookup(tlink)
	}
	return nil
}

// RequiredUIs returns the required UIs.
func (e *Identify2) RequiredUIs() []libkb.UIKind {
	return []libkb.UIKind{
		libkb.IdentifyUIKind,
	}
}

// SubConsumers returns the other UI consumers for this engine.
func (e *Identify2) SubConsumers() []libkb.UIConsumer {
	return nil
}

func (e *Identify2) isSelfLoad() bool {
	return e.me != nil && e.them != nil && e.me.Equal(e.them)
}

func (e *Identify2) loadMe(ctx *Context) (err error) {
	var ok bool
	ok, err = IsLoggedIn(e, ctx)
	if err != nil || !ok {
		return err
	}
	e.me, err = libkb.LoadMe(libkb.NewLoadUserArg(e.G()))
	return err
}

func (e *Identify2) loadThem(ctx *Context) (err error) {
	arg := libkb.NewLoadUserArg(e.G())
	arg.UID = e.arg.Uid
	e.them, err = libkb.LoadUser(arg)
	if e.them == nil {
		panic("Expected e.them to be true after successful loadUser")
	}
	return err
}

func (e *Identify2) loadUsers(ctx *Context) (err error) {
	if err = e.loadMe(ctx); err != nil {
		return err
	}
	if err = e.loadThem(ctx); err != nil {
		return err
	}
	return nil
}

func (e *Identify2) checkFastCacheHit() bool {
	u, _ := e.G().UserCache.Get(e.them.GetUID())
	if u == nil {
		return false
	}
	then := u.Uvv.CachedAt
	if then == 0 {
		return false
	}
	thenTime := time.Unix(int64(then), 0)
	if time.Now().Sub(thenTime) > libkb.IdentifyCacheLongTimeout {
		return false
	}
	e.cachedRes = u
	return true
}

func (e *Identify2) checkSlowCacheHit() bool {

	u, _ := e.G().UserCache.Get(e.them.GetUID())
	if u == nil {
		return false
	}
	if !e.them.IsCachedIdentifyFresh(u) {
		return false
	}
	e.cachedRes = u
	return true
}

func (e *Identify2) Result() *keybase1.Identify2Res {
	res := &keybase1.Identify2Res{}
	if e.cachedRes != nil {
		res.Upk = *e.cachedRes
	} else if e.them != nil {
		res.Upk = e.toUserPlusKeys()
	}
	return res
}

func (e *Identify2) toUserPlusKeys() keybase1.UserPlusKeys {
	return e.them.ExportToUserPlusKeys(e.getIdentifyTime())
}
