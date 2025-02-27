package fuse

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"github.com/libopenstorage/openstorage/api"
	"github.com/libopenstorage/openstorage/volume"
	"github.com/libopenstorage/openstorage/volume/drivers/common"
	"github.com/pborman/uuid"
	"github.com/portworx/kvdb"
)

type volumeDriver struct {
	volume.IODriver
	volume.BlockDriver
	volume.SnapshotDriver
	volume.StoreEnumerator
	volume.StatsDriver
	volume.QuiesceDriver
	volume.CredsDriver
	volume.CloudBackupDriver
	volume.CloudMigrateDriver
	volume.FilesystemTrimDriver
	volume.FilesystemCheckDriver
	name        string
	baseDirPath string
	provider    Provider
}

func newVolumeDriver(
	name string,
	baseDirPath string,
	provider Provider,
) *volumeDriver {
	return &volumeDriver{
		volume.IONotSupported,
		volume.BlockNotSupported,
		volume.SnapshotNotSupported,
		common.NewDefaultStoreEnumerator(
			name,
			kvdb.Instance(),
		),
		volume.StatsNotSupported,
		volume.QuiesceNotSupported,
		volume.CredsNotSupported,
		volume.CloudBackupNotSupported,
		volume.CloudMigrateNotSupported,
		volume.FilesystemTrimNotSupported,
		volume.FilesystemCheckNotSupported,
		name,
		baseDirPath,
		provider,
	}
}

func (v *volumeDriver) GetVolumeWatcher(locator *api.VolumeLocator, labels map[string]string) (chan *api.Volume, error) {
	return nil, nil
}

func (v *volumeDriver) Name() string {
	return v.name
}

func (v *volumeDriver) Type() api.DriverType {
	return api.DriverType_DRIVER_TYPE_FILE
}

func (v *volumeDriver) Version() (*api.StorageVersion, error) {
	return &api.StorageVersion{
		Driver:  v.Name(),
		Version: "1.0.0",
	}, nil
}

func (v *volumeDriver) Create(
	ctx context.Context,
	volumeLocator *api.VolumeLocator,
	source *api.Source,
	spec *api.VolumeSpec,
) (string, error) {
	volumeID := strings.TrimSpace(string(uuid.New()))
	dirPath := filepath.Join(v.baseDirPath, volumeID)
	if err := os.MkdirAll(dirPath, 0777); err != nil {
		return "", err
	}
	volume := common.NewVolume(
		volumeID,
		api.FSType_FS_TYPE_FUSE,
		volumeLocator,
		source,
		spec,
	)
	volume.DevicePath = dirPath
	if err := v.CreateVol(volume); err != nil {
		return "", err
	}
	if err := v.UpdateVol(volume); err != nil {
		return "", err
	}
	return volume.Id, nil
}

func (v *volumeDriver) Delete(ctx context.Context, volumeID string) error {
	if _, err := v.GetVol(volumeID); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(v.baseDirPath, string(volumeID))); err != nil {
		return err
	}
	return v.DeleteVol(volumeID)
}

func (v *volumeDriver) MountedAt(ctx context.Context, mountpath string) string {
	return ""
}

func (v *volumeDriver) Mount(ctx context.Context, volumeID string, mountpath string, options map[string]string) error {
	volume, err := v.GetVol(volumeID)
	if err != nil {
		return err
	}
	if len(volume.AttachPath) > 0 && len(volume.AttachPath) > 0 {
		return fmt.Errorf("Volume %q already mounted at %q", volumeID, volume.AttachPath[0])
	}
	mountOptions, err := v.provider.GetMountOptions(volume.Spec)
	if err != nil {
		return err
	}
	conn, err := fuse.Mount(mountpath, mountOptions...)
	if err != nil {
		return err
	}
	filesystem, err := v.provider.GetFS(volume.Spec)
	if err != nil {
		return err
	}
	go func() {
		// TODO: track error once we understand driver model better
		_ = fs.Serve(conn, filesystem)
		_ = conn.Close()
	}()
	<-conn.Ready
	if conn.MountError == nil {
		if volume.AttachPath == nil {
			volume.AttachPath = make([]string, 1)
		}
		volume.AttachPath[0] = mountpath
	}
	return conn.MountError
}

func (v *volumeDriver) Unmount(ctx context.Context, volumeID string, mountpath string, options map[string]string) error {
	volume, err := v.GetVol(volumeID)
	if err != nil {
		return err
	}
	if len(volume.AttachPath) == 0 || len(volume.AttachPath[0]) == 0 {
		return fmt.Errorf("Device %v not mounted", volumeID)
	}
	if err := fuse.Unmount(volume.AttachPath[0]); err != nil {
		return err
	}
	volume.AttachPath = nil
	return v.UpdateVol(volume)
}

func (v *volumeDriver) Set(volumeID string, locator *api.VolumeLocator, spec *api.VolumeSpec) error {
	return volume.ErrNotSupported

}

func (v *volumeDriver) Status() [][2]string {
	return [][2]string{}
}

func (v *volumeDriver) Shutdown() {}

func (d *volumeDriver) Catalog(volumeID, path, depth string) (api.CatalogResponse, error) {
	return api.CatalogResponse{}, volume.ErrNotSupported
}

func (d *volumeDriver) VolService(volumeID string, vtreq *api.VolumeServiceRequest) (*api.VolumeServiceResponse, error) {
	return nil, volume.ErrNotSupported
}
