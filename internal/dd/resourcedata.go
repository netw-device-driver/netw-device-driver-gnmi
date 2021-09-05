package dd

import (
	"github.com/netw-device-driver/ndd-runtime/pkg/gext"
	"github.com/openconfig/gnmi/proto/gnmi"
)

type resourceData struct {
	Name    string
	Level   int
	Path    *gnmi.Path // is the first entry of the update
	Replace []*gnmi.Update
	Status  gext.ResourceStatus
}

func (r *resourceData) GetName() string {
	return r.Name
}

func (r *resourceData) GetLevel() int {
	return r.Level
}

func (r *resourceData) GetPath() *gnmi.Path {
	return r.Path
}

func (r *resourceData) GetUpdate() []*gnmi.Update {
	return r.Replace
}

func (r *resourceData) GetStatus() gext.ResourceStatus {
	return r.Status
}

func (r *resourceData) SetName(s string) {
	r.Name = s
}

func (r *resourceData) SetLevel(s int) {
	r.Level = s
}

func (r *resourceData) SetPath(s *gnmi.Path) {
	r.Path = s
}

func (r *resourceData) SetUpdate(s []*gnmi.Update) {
	r.Replace = s
}

func (r *resourceData) SetStatus(s gext.ResourceStatus) {
	r.Status = s
}
