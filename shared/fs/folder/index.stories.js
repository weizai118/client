// @flow
import * as I from 'immutable'
import React from 'react'
import * as Types from '../../constants/types/fs'
import * as Sb from '../../stories/storybook'
import DestinationPicker from './destination-picker'
import Folder from '.'
import * as Kb from '../../common-adapters'
import {isMobile} from '../../constants/platform'
import {rowsProvider} from './rows/index.stories'
import {commonProvider} from '../common/index.stories'
import {topBarProvider} from '../top-bar/index.stories'
import {footerProvider} from '../footer/index.stories'
import {bannerProvider} from '../banner/index.stories'

const provider = Sb.createPropProviderWithCommon({
  ...rowsProvider,
  ...commonProvider,
  ...topBarProvider,
  ...footerProvider,
  ...bannerProvider,

  // for DestinationPicker
  NavHeaderTitle: ({path}: {path: Types.Path}) => ({
    onOpenPath: Sb.action('onOpenPath'),
    path,
  }),
})

export default () => {
  Sb.storiesOf('Files/Folder', module)
    .addDecorator(provider)
    .add('normal', () => (
      <Kb.Box2 direction="horizontal" fullWidth={true} fullHeight={true}>
        <Folder
          path={Types.stringToPath('/keybase/private/foo')}
          routePath={I.List([])}
          shouldShowSFMIBanner={false}
          resetBannerType="none"
          offline={false}
        />
      </Kb.Box2>
    ))
    .add('with SystemFileManagerIntegrationBanner', () => (
      <Kb.Box2 direction="horizontal" fullWidth={true} fullHeight={true}>
        <Folder
          path={Types.stringToPath('/keybase/private/foo')}
          routePath={I.List([])}
          shouldShowSFMIBanner={true}
          resetBannerType="none"
          offline={false}
        />
      </Kb.Box2>
    ))
    .add('I am reset', () => (
      <Kb.Box2 direction="horizontal" fullWidth={true} fullHeight={true}>
        <Folder
          path={Types.stringToPath('/keybase/private/me,reset')}
          routePath={I.List([])}
          shouldShowSFMIBanner={false}
          resetBannerType="self"
          offline={false}
        />
      </Kb.Box2>
    ))
    .add('others reset', () => (
      <Kb.Box2 direction="horizontal" fullWidth={true} fullHeight={true}>
        <Folder
          path={Types.stringToPath('/keybase/private/others,reset')}
          routePath={I.List([])}
          shouldShowSFMIBanner={false}
          resetBannerType={1}
          offline={false}
        />
      </Kb.Box2>
    ))
    .add('offline and not synced', () => (
      <Kb.Box2 direction="horizontal" fullWidth={true} fullHeight={true}>
        <Folder
          path={Types.stringToPath('/keybase/private/others,reset')}
          routePath={I.List([])}
          shouldShowSFMIBanner={false}
          resetBannerType="none"
          offline={true}
        />
      </Kb.Box2>
    ))
  Sb.storiesOf('Files', module)
    .addDecorator(provider)
    .add('DestinationPicker', () => (
      <DestinationPicker
        parentPath={Types.stringToPath('/keybase/private/meatball,songgao,xinyuzhao/yo')}
        routePath={I.List([])}
        onCancel={Sb.action('onCancel')}
        targetName="Secret treat spot blasjeiofjawiefjksadjflaj long name blahblah"
        index={0}
        onCopyHere={Sb.action('onCopyHere')}
        onMoveHere={Sb.action('onMoveHere')}
        onNewFolder={Sb.action('onNewFolder')}
        onBackUp={isMobile ? Sb.action('onBackUp') : null}
      />
    ))
}
