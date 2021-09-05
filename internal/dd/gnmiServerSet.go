package dd

import (
	"context"
	"encoding/json"
	"time"

	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/gext"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	"github.com/pkg/errors"
	yparser "github.com/yndd/ndd-yang/pkg/parser"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Set(ctx context.Context, req *gnmi.SetRequest) (*gnmi.SetResponse, error) {
	ok := s.unaryRPCsem.TryAcquire(1)
	if !ok {
		return nil, status.Errorf(codes.ResourceExhausted, "max number of Unary RPC reached")
	}
	defer s.unaryRPCsem.Release(1)

	numUpdates := len(req.GetUpdate())
	numReplaces := len(req.GetReplace())
	numDeletes := len(req.GetDelete())
	if numUpdates+numReplaces+numDeletes == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "missing update/replace/delete path(s)")
	}

	extensions := make(map[gnmi_ext.ExtensionID]string)
	for _, ext := range req.GetExtension() {
		extensions[ext.GetRegisteredExt().GetId()] = string(ext.GetRegisteredExt().GetMsg())
	}

	gextInfo := &gext.GEXT{}
	var err error
	if ext, ok := extensions[gnmi_ext.ExtensionID_EID_EXPERIMENTAL]; ok {
		//s.log.Debug("GNMI Get global entry", "GEXT Info", ext)
		gextInfo, err = gext.String2GEXT(ext)
		if err != nil {
			return nil, err
		}
	}

	//s.log.Debug("GNMI Set global entry", "Extension", extensions, "GEXT Info", gextInfo)

	switch {
	case gextInfo.Action == gext.GEXTActionCreate:
		// Create
		if numDeletes != 0 || numUpdates != 0 {
			return nil, status.Errorf(codes.InvalidArgument, "a create can only have replace objects")
		}
		// CreateGNMIK8sResource creates a K8s resource
		rsp, err := s.Cache.CreateGNMIK8sResource(ctx, req, gextInfo)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	case gextInfo.Action == gext.GEXTActionDelete:
		// Delete
		if numReplaces != 0 || numUpdates != 0 {
			return nil, status.Errorf(codes.InvalidArgument, "a delete can only have delete objects")
		}
		// DeleteGNMIK8sResource creates a K8s resource
		rsp, err := s.Cache.DeleteGNMIK8sResource(ctx, req, gextInfo)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	case numReplaces != 0 && req.Replace[0].GetPath().GetElem()[0].GetName() == nddv1.RegisterPathElemName:
		// this a special case for Register
		rsp, err := s.Cache.CreateRegister(ctx, req)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	case numUpdates != 0 && req.Update[0].GetPath().GetElem()[0].GetName() == nddv1.RegisterPathElemName:
		// this a special case for Register
		rsp, err := s.Cache.UpdateRegister(ctx, req)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	case numDeletes != 0 && req.Delete[0].GetElem()[0].GetName() == nddv1.RegisterPathElemName:
		// this a special case for Register
		rsp, err := s.Cache.DeleteRegister(ctx, req)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	default:
		// Update
		// UpdateGNMIK8sResource updates the paths of a K8s resource
		rsp, err := s.Cache.UpdateGNMIK8sResource(ctx, req, gextInfo)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	}

}

// Create creates a managed resource from the provider
func (c *Cache) CreateGNMIK8sResource(ctx context.Context, req *gnmi.SetRequest, gextInfo *gext.GEXT) (*gnmi.SetResponse, error) {
	log := c.log.WithValues("Action", gextInfo.GetAction(), "ResourceName", gextInfo.GetName(), "Replace", req.GetReplace())
	log.Debug("Create K8sResource...")
	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &gnmi.SetResponse{
			Response: []*gnmi.UpdateResult{
				{
					Timestamp: time.Now().UnixNano(),
				},
			}}, nil
	}
	c.Lock()
	if _, ok := c.Data2[gextInfo.GetLevel()]; !ok {
		c.Data2[gextInfo.GetLevel()] = make(map[string]*resourceData)
	}
	log.Debug("CreateGNMIK8sResource", "ResourcePath", c.parser.GnmiPathToXPath(req.GetReplace()[0].GetPath(), true))
	c.Data2[gextInfo.GetLevel()][gextInfo.GetName()] = &resourceData{
		Name:    gextInfo.GetName(),
		Level:   gextInfo.GetLevel(),
		Path:    gextInfo.RootPath, // path is the first entry of the replace object in the gnmi
		Replace: req.GetReplace(),
		Status:  gext.ResourceStatusCreatePending,
	}
	c.SetNewProviderUpdates(true)
	c.Unlock()

	c.ShowStatus()

	log.Debug("Config Create reply...")
	return &gnmi.SetResponse{
		Response: []*gnmi.UpdateResult{
			{
				Timestamp: time.Now().UnixNano(),
			},
		}}, nil
}

// Delete deletes a managed resource from the provider
func (c *Cache) DeleteGNMIK8sResource(ctx context.Context, req *gnmi.SetRequest, gextInfo *gext.GEXT) (*gnmi.SetResponse, error) {
	log := c.log.WithValues("Action", gextInfo.GetAction(), "ResourceName", gextInfo.GetName(), "Delete", req.GetDelete())
	log.Debug("Delete K8sResource...", "Cache", c.Data2)
	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &gnmi.SetResponse{
			Response: []*gnmi.UpdateResult{
				{
					Timestamp: time.Now().UnixNano(),
				},
			}}, nil
	}

	if _, ok := c.Data2[gextInfo.GetLevel()]; ok {
		if _, ok := c.Data2[gextInfo.GetLevel()][gextInfo.GetName()]; ok {
			c.Data2[gextInfo.GetLevel()][gextInfo.GetName()].Status = gext.ResourceStatusDeletePending
			c.Lock()
			c.SetNewProviderUpdates(true)
			c.Unlock()
			log.Debug("Config Delete reply, resource found set status to delete pending", "level", gextInfo.GetLevel(), "Name", gextInfo.GetName())
		} else {
			log.Debug("Config Delete reply resourcename not found", "level", gextInfo.GetLevel(), "Name", gextInfo.GetName())
		}
	} else {
		log.Debug("Config Delete reply level not found", "level", gextInfo.GetLevel(), "Name", gextInfo.GetName())
	}
	return &gnmi.SetResponse{
		Response: []*gnmi.UpdateResult{
			{
				Timestamp: time.Now().UnixNano(),
			},
		}}, nil
}

// Update updates a managed resource from the provider
func (c *Cache) UpdateGNMIK8sResource(ctx context.Context, req *gnmi.SetRequest, gextInfo *gext.GEXT) (*gnmi.SetResponse, error) {
	log := c.log.WithValues("Action", gextInfo.GetAction(), "ResourceName", gextInfo.GetName(), "Delete", req.GetDelete(), "Update", req.GetUpdate())
	log.Debug("Update K8sResource...")

	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &gnmi.SetResponse{
			Response: []*gnmi.UpdateResult{
				{
					Timestamp: time.Now().UnixNano(),
				},
			}}, nil
	}
	// initialize a tracecontext
	tc := &yparser.TraceCtxtGnmi{}

	// we update the changes in a single transaction
	_, err := c.Device.SetGnmi(ctx, req.Update, req.Delete)
	if err != nil {
		// delete not successfull
		// TODO we should fail in certain conditions
		c.log.Debug("GNMI Set failed", "Paths", req.Delete, "Error", err)
		return nil, err
	}
	// delete was successfull, update the Resource cache
	for _, delPath := range req.Delete {
		log.Debug("GNMI delete success", "Path", delPath)
		c.Lock()
		c.CurrentConfig, tc = c.DeleteObjectGnmi(c.CurrentConfig, delPath)
		c.Unlock()
		if !tc.Found {
			log.WithValues(
				"Cache Resource", gextInfo.GetName(),
				"Xpath", delPath,
			).Debug("ReconcileUpdate Delete Not found", "tc", tc.Idx, "tc.msg", tc.Msg, "tc.path", tc.Path)
		}
	}

	// update was successfull, update the Resource cache
	for _, update := range req.Update {
		log.Debug("GNMI update success", "ResourceName", gextInfo.GetName(), "Path", update.Path)

		x1, err := c.parser.GetValue(update.GetVal())
		if err != nil {
			c.log.Debug("Cannot parse value", "Error", err)
			return nil, err
		}

		c.Lock()
		c.CurrentConfig, tc = c.UpdateObjectGnmi(c.CurrentConfig, update.Path, x1)
		log.Debug("Update in Cache", "Result", tc)
		c.Unlock()
		if !tc.Found {
			log.WithValues(
				"Cache Resource", gextInfo.GetName(),
				"Xpath", update.Path,
			).Debug("ReconcileUpdate Not found", "tc", tc.Idx, "tc.msg", tc.Msg, "tc.path", tc.Path)
		}
	}

	log.Debug("Update K8sResource reply...")
	return &gnmi.SetResponse{
		Response: []*gnmi.UpdateResult{
			{
				Timestamp: time.Now().UnixNano(),
			},
		}}, nil
}

// Create the registration
func (c *Cache) CreateRegister(ctx context.Context, req *gnmi.SetRequest) (*gnmi.SetResponse, error) {
	log := c.log.WithValues("Replace", req.GetReplace())
	log.Debug("Create Register...")

	// we dont need to check the PathElem since this was done to get here
	// we still need to validate the PathElemKeyName
	key := req.Replace[0].GetPath().GetElem()[0].GetKey()
	if _, ok := key["name"]; !ok {
		return nil, errors.New("Devictype key not provided")
	}
	deviceType := key["name"]
	//register := &nddv1.Register{}

	value, err := c.parser.GetValue(req.Replace[0].GetVal())
	if err != nil {
		return nil, err
	}

	log.Debug("Create Register...", "Data1", value)
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	log.Debug("Create Register...", "Data2", string(b))
	r := &nddv1.Register{}
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	log.Debug("Create Register...", "Data3", r)

	c.RegisteredDevices[nddv1.DeviceType(deviceType)] = r
	log.Debug("Create Register reply...")
	return &gnmi.SetResponse{
		Response: []*gnmi.UpdateResult{
			{
				Timestamp: time.Now().UnixNano(),
			},
		}}, nil
}

// Update the registration
func (c *Cache) UpdateRegister(ctx context.Context, req *gnmi.SetRequest) (*gnmi.SetResponse, error) {
	log := c.log.WithValues("Update", req.GetUpdate())
	log.Debug("Update Register...")

	// we dont need to check the element as this was already done
	// we need to validate the key
	key := req.Update[0].GetPath().GetElem()[0].GetKey()
	if _, ok := key["name"]; !ok {
		return nil, errors.New("Devictype key not provided")
	}
	deviceType := key["name"]

	value, err := c.parser.GetValue(req.Replace[0].GetVal())
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	r := &nddv1.Register{}
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}

	c.RegisteredDevices[nddv1.DeviceType(deviceType)] = r

	log.Debug("Update Register reply...")
	return &gnmi.SetResponse{
		Response: []*gnmi.UpdateResult{
			{
				Timestamp: time.Now().UnixNano(),
			},
		}}, nil
}

// Delete the registration
func (c *Cache) DeleteRegister(ctx context.Context, req *gnmi.SetRequest) (*gnmi.SetResponse, error) {
	log := c.log.WithValues("Path", req.GetDelete())
	log.Debug("Delte Register...")

	// we dont need to check the element as this was already done
	// we need to validate the key
	key := req.Delete[0].GetElem()[0].GetKey()
	if _, ok := key["name"]; !ok {
		return nil, errors.New("Devictype key not provided")
	}
	deviceType := key["name"]

	delete(c.RegisteredDevices, nddv1.DeviceType(deviceType))

	log.Debug("Delete Register reply...")
	return &gnmi.SetResponse{
		Response: []*gnmi.UpdateResult{
			{
				Timestamp: time.Now().UnixNano(),
			},
		}}, nil
}
