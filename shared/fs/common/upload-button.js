// @flow
import {namedConnect} from '../../util/container'
import * as Types from '../../constants/types/fs'
import * as Constants from '../../constants/fs'
import * as Kb from '../../common-adapters'
import * as FsGen from '../../actions/fs-gen'
import * as React from 'react'
import * as Styles from '../../styles'
import * as Platforms from '../../constants/platform'

type OwnProps = {|
  path: Types.Path,
  style?: ?Styles.StylesCrossPlatform,
|}

const UploadButton = Kb.OverlayParentHOC(props => {
  if (!props.canUpload) {
    return null
  }
  if (props.openAndUploadBoth) {
    return <Kb.Button small={true} onClick={props.openAndUploadBoth} label="Upload" style={props.style} />
  }
  if (props.pickAndUploadMixed) {
    return <Kb.Icon type="iconfont-upload" padding="tiny" onClick={props.pickAndUploadMixed} />
  }
  // Either Android, or non-darwin desktop. Android doesn't support mixed
  // mode; Linux/Windows don't support opening file or dir from the same
  // dialog. In both cases a menu is needed.
  return (
    <>
      {Platforms.isMobile ? (
        <Kb.Icon type="iconfont-upload" padding="tiny" onClick={props.toggleShowingMenu} />
      ) : (
        <Kb.Button
          onClick={props.toggleShowingMenu}
          label="Upload"
          ref={props.setAttachmentRef}
          style={props.style}
        />
      )}
      <Kb.FloatingMenu
        attachTo={props.getAttachmentRef}
        visible={props.showingMenu}
        onHidden={props.toggleShowingMenu}
        items={[
          ...(props.pickAndUploadPhoto
            ? [
                {
                  onClick: props.pickAndUploadPhoto,
                  title: 'Upload photo',
                },
              ]
            : []),
          ...(props.pickAndUploadVideo
            ? [
                {
                  onClick: props.pickAndUploadVideo,
                  title: 'Upload video',
                },
              ]
            : []),
          ...(props.openAndUploadDirectory
            ? [
                {
                  onClick: props.openAndUploadDirectory,
                  title: 'Upload directory',
                },
              ]
            : []),
          ...(props.openAndUploadFile
            ? [
                {
                  onClick: props.openAndUploadFile,
                  title: 'Upload file',
                },
              ]
            : []),
        ]}
        position="bottom left"
        closeOnSelect={true}
      />
    </>
  )
})

const mapStateToProps = (state, {path}) => ({
  _pathItem: state.fs.pathItems.get(path, Constants.unknownPathItem),
})

const mapDispatchToProps = (dispatch, {path}) => ({
  openAndUploadBoth: Platforms.isDarwin
    ? () => dispatch(FsGen.createOpenAndUpload({parentPath: path, type: 'both'}))
    : null,
  openAndUploadDirectory:
    Platforms.isElectron && !Platforms.isDarwin
      ? () => dispatch(FsGen.createOpenAndUpload({parentPath: path, type: 'directory'}))
      : null,
  openAndUploadFile:
    Platforms.isElectron && !Platforms.isDarwin
      ? () => dispatch(FsGen.createOpenAndUpload({parentPath: path, type: 'file'}))
      : null,
  pickAndUploadMixed: Platforms.isIOS
    ? () => dispatch(FsGen.createPickAndUpload({parentPath: path, type: 'mixed'}))
    : null,
  pickAndUploadPhoto: Platforms.isAndroid
    ? () => dispatch(FsGen.createPickAndUpload({parentPath: path, type: 'photo'}))
    : null,
  pickAndUploadVideo: Platforms.isAndroid
    ? () => dispatch(FsGen.createPickAndUpload({parentPath: path, type: 'video'}))
    : null,
})

const mergeProps = (s, d, o) => ({
  canUpload: s._pathItem.type === 'folder' && s._pathItem.writable,
  style: o.style,
  ...d,
})

export default namedConnect<OwnProps, _, _, _, _>(
  mapStateToProps,
  mapDispatchToProps,
  mergeProps,
  'UploadButton'
)(UploadButton)
