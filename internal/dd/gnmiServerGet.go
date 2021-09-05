package dd

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/gext"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Get(ctx context.Context, req *gnmi.GetRequest) (*gnmi.GetResponse, error) {
	ok := s.unaryRPCsem.TryAcquire(1)
	if !ok {
		return nil, status.Errorf(codes.ResourceExhausted, "max number of Unary RPC reached")
	}
	defer s.unaryRPCsem.Release(1)

	// The device driver uses gnmi extension to distinguish the different GetRequest
	// There are 2 kinds of GetRequests,
	// 1. to resolve the resourceName (GKV of k8s for external leafref validation)
	// 2. a reqular GetRequest
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

	//s.log.Debug("GNMI Get global entry", "Extension", extensions, "GEXT Info", gextInfo)

	switch {
	case gextInfo.Action == gext.GEXTActionGetResourceName:
		// GetRequest to resolve the ResourceName
		rsp, err := s.Cache.GetGNMIK8sResourceName(ctx, req, gextInfo)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	case gextInfo.Action == gext.GEXTActionGet:
		// GetRequest to resolve the ResourceName
		rsp, err := s.Cache.GetGNMIK8s(ctx, req, gextInfo)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	case len(req.GetPath()) != 0 && len(req.GetPath()[0].GetElem()) != 0 && req.GetPath()[0].GetElem()[0].GetName() == nddv1.RegisterPathElemName:
		// this a special case for Register
		rsp, err := s.Cache.GetRegister(ctx, req)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	default:
		// Regular Get Request
		rsp, err := s.Cache.GetGNMI(ctx, req)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	}
}

// GetResourceName returns the matched resource from the path, by checking the best match from the cache
func (c *Cache) GetGNMIK8sResourceName(ctx context.Context, req *gnmi.GetRequest, gextInfo *gext.GEXT) (*gnmi.GetResponse, error) {
	log := c.log.WithValues("Action", gextInfo.GetAction(), "ResourceName", gextInfo.GetName(), "Path", req.GetPath())
	log.Debug("K8sResource GetResourceName...")
	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		// we should never come here
		log.Debug("K8sResource GetResourceName Cache not ready, the reconcilation loop should avoid the code ever coming here")
		d, err := json.Marshal(nddv1.ResourceName{
			Name: "",
		})
		if err != nil {
			return &gnmi.GetResponse{}, err
		}
		updates := make([]*gnmi.Update, 0)
		upd := &gnmi.Update{
			Path: req.GetPath()[0],
			Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: d}},
		}
		updates = append(updates, upd)
		return getResponse(req, req.GetExtension(), updates), nil
	}
	// provide a string from the gnmi Path, we expect a single path in the GetRequest
	reqPath := *c.parser.GnmiPathToXPath(req.GetPath()[0], true)
	// initialize the variables which are used to keep track of the matched strings
	matchedResourceName := ""
	matchedResourcePath := ""
	// loop over the cache
	for _, resourceData := range c.Data2 {
		for resourceName, resourceStatus := range resourceData {
			// TO BE UPDATED
			resourcePath := *c.parser.GnmiPathToXPath(resourceStatus.Path, true)
			// check if the string is contained in the path
			if strings.Contains(reqPath, resourcePath) {
				// if there is a better match we use the better match
				if len(resourcePath) > len(matchedResourcePath) {
					matchedResourcePath = resourcePath
					matchedResourceName = resourceName
				}
			}
		}
	}
	log.Debug("K8sResource GetResourceName Match", "ResourceName", matchedResourceName)

	d, err := json.Marshal(nddv1.ResourceName{
		Name: matchedResourceName,
	})
	if err != nil {
		return getResponse(req, req.GetExtension(), nil), err
	}
	updates := make([]*gnmi.Update, 0)
	upd := &gnmi.Update{
		Path: req.GetPath()[0],
		Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: d}},
	}
	updates = append(updates, upd)
	return getResponse(req, req.GetExtension(), updates), nil

}

func getGnmiExtension(name string, status gext.ResourceStatus, Exists, HasData, Ready bool) ([]*gnmi_ext.Extension, error) {
	gextInfo := &gext.GEXT{
		Name:       name,
		Status:     status,
		Exists:     Exists,
		HasData:    HasData,
		CacheReady: Ready,
	}
	gextByte, err := json.Marshal(gextInfo)
	if err != nil {
		return nil, err
	}

	return []*gnmi_ext.Extension{
		{Ext: &gnmi_ext.Extension_RegisteredExt{
			RegisteredExt: &gnmi_ext.RegisteredExtension{Id: gnmi_ext.ExtensionID_EID_EXPERIMENTAL, Msg: gextByte}}},
	}, nil
}

func getResponse(req *gnmi.GetRequest, ext []*gnmi_ext.Extension, upd []*gnmi.Update) *gnmi.GetResponse {
	return &gnmi.GetResponse{
		Notification: []*gnmi.Notification{
			{
				Timestamp: time.Now().UnixNano(),
				Prefix:    req.GetPrefix(),
				Update:    upd,
			},
		},
		Extension: ext,
	}
}

// GetK8sGNMI returns the data match from the k8s resource
func (c *Cache) GetGNMIK8s(ctx context.Context, req *gnmi.GetRequest, gextInfo *gext.GEXT) (*gnmi.GetResponse, error) {
	log := c.log.WithValues("Action", gextInfo.GetAction(), "ResourceName", gextInfo.GetName(), "Path", req.GetPath())
	log.Debug("Get K8sResource...")

	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		ext, err := getGnmiExtension("", gext.ResourceStatusNone, false, false, false)
		log.Debug("Cache Not ready", "ext", ext)
		if err != nil {
			return nil, err
		}
		return getResponse(req, ext, nil), nil
	}
	// Get data from currentconfig in cache, we assume a single path in the Get
	value, tc := c.GetObjectValueGnmi(c.CurrentConfig, req.GetPath()[0])
	if !tc.Found {
		log.Debug("Config Get Data not found", "Trace Context", tc)
		if _, ok := c.Data2[int(gextInfo.GetLevel())]; ok {
			if x, ok := c.Data2[int(gextInfo.GetLevel())][gextInfo.GetName()]; ok {
				// this is a managed resource and there is no data
				// this is a bit strange if we come here
				c.Lock()
				ext, err := getGnmiExtension(x.Name, x.Status, true, false, true)
				c.Unlock()
				if err != nil {
					return nil, err
				}
				return getResponse(req, ext, nil), nil
			} else {
				// there is no managed resource info and there is no data as an unmanaged resource
				ext, err := getGnmiExtension("", gext.ResourceStatusNone, false, false, true)
				if err != nil {
					return nil, err
				}
				return getResponse(req, ext, nil), nil
			}
		} else {
			// there is no managed resource info and there is no data as an unmanaged resource
			ext, err := getGnmiExtension("", gext.ResourceStatusNone, false, false, true)
			if err != nil {
				return nil, err
			}
			return getResponse(req, ext, nil), nil

		}
	} else {
		// data exists
		// copy the data to a new struct
		x1, err := c.parser.DeepCopy(value)
		if err != nil {
			log.Debug("Config Get/Observe cannot copy data", "error", err)
			ext, err := getGnmiExtension("", gext.ResourceStatusNone, true, false, true)
			if err != nil {
				return nil, err
			}
			return getResponse(req, ext, nil), err
		}
		log.Debug("Config Get/Observe reply from cache", "data", x1)

		if _, ok := c.Data2[int(gextInfo.GetLevel())]; ok {
			if x, ok := c.Data2[int(gextInfo.GetLevel())][gextInfo.GetName()]; ok {
				// this is a managed resource and there is data, clean it before
				// sending the response to the provider:
				// Remove the PathElems of a hierarchical resource
				// find all subPaths that have a hierarchical dependency on the main resource
				d, err := c.ParseReturnObserveDataGnmi(x, x1)
				if err != nil {
					ext, err := getGnmiExtension(x.Name, x.Status, true, false, true)
					if err != nil {
						return nil, err
					}
					return getResponse(req, ext, nil), err
				}
				// return data
				ext, err := getGnmiExtension(x.Name, x.Status, true, true, true)
				if err != nil {
					return nil, err
				}
				updates := make([]*gnmi.Update, 0)
				upd := &gnmi.Update{
					Path: req.GetPath()[0],
					Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: d}},
				}
				updates = append(updates, upd)
				return getResponse(req, ext, updates), nil
			} else {
				// there is no managed resource info and there is data as an unmanaged resource
				// not sure if we ever come here
				d, err := json.Marshal(x1)
				if err != nil {
					// error marshaling the data, return info we have with error
					ext, err := getGnmiExtension("", gext.ResourceStatusNone, false, false, true)
					if err != nil {
						return nil, err
					}
					return getResponse(req, ext, nil), err
				}
				ext, err := getGnmiExtension("", gext.ResourceStatusNone, false, true, true)
				if err != nil {
					return nil, err
				}

				updates := make([]*gnmi.Update, 0)
				upd := &gnmi.Update{
					Path: req.GetPath()[0],
					Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: d}},
				}
				updates = append(updates, upd)
				return getResponse(req, ext, updates), nil
			}
		} else {
			// there is no managed resource info and there is data as an unmanaged resource
			// not sure if we ever come here
			d, err := json.Marshal(x1)
			if err != nil {
				// error marshaling the data, return info we have with error
				ext, err := getGnmiExtension("", gext.ResourceStatusNone, false, false, true)
				if err != nil {
					return nil, err
				}
				return getResponse(req, ext, nil), err
			}
			ext, err := getGnmiExtension("", gext.ResourceStatusNone, false, true, true)
			if err != nil {
				return nil, err
			}
			updates := make([]*gnmi.Update, 0)
			upd := &gnmi.Update{
				Path: req.GetPath()[0],
				Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: d}},
			}
			updates = append(updates, upd)
			return getResponse(req, ext, updates), nil

		}
	}
	//return nil, nil
}

// GetGNMI returns the data match from the path
func (c *Cache) GetGNMI(ctx context.Context, req *gnmi.GetRequest) (*gnmi.GetResponse, error) {
	log := c.log.WithValues("Path", req.GetPath())
	log.Debug("Get GNMI...")

	// Cache readiness should not be checked here since the register uses a regular get to check the
	// registration, if the cache ready check would be done here we end up in a deadlock where the
	// registration does not succeed.

	updates := make([]*gnmi.Update, 0)
	if req.GetPath() == nil {
		// when there is no path info we return the current config
		d, err := json.Marshal(c.CurrentConfig)
		if err != nil {
			c.log.Debug("Config Marshal datacopy data", "error", err)
			return getResponse(req, req.GetExtension(), nil), err
		}
		upd := &gnmi.Update{
			Path: &gnmi.Path{},
			Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: d}},
		}
		updates = append(updates, upd)
	} else {
		for _, path := range req.GetPath() {
			value, tc := c.GetObjectValueGnmi(c.CurrentConfig, path)
			if !tc.Found {
				log.Debug("Config Get Data not found", "Trace Context", tc)

			} else {
				// data exists
				// copy the data to a new struct
				x1, err := c.parser.DeepCopy(value)
				if err != nil {
					log.Debug("Config Get/Observe cannot copy data", "error", err)
					return getResponse(req, req.GetExtension(), nil), err
				}
				log.Debug("Get GNMI reply from cache", "data", x1)
				d, err := json.Marshal(x1)
				if err != nil {
					c.log.Debug("Config Marshal datacopy data", "error", err)
					return getResponse(req, req.GetExtension(), nil), err
				}
				upd := &gnmi.Update{
					Path: path,
					Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: d}},
				}
				updates = append(updates, upd)
			}
		}
	}

	return getResponse(req, req.GetExtension(), updates), nil
}

// GetGNMI returns the data match from the path
func (c *Cache) GetRegister(ctx context.Context, req *gnmi.GetRequest) (*gnmi.GetResponse, error) {
	log := c.log.WithValues("Path", req.GetPath())
	log.Debug("Get Register...")

	// we dont need to check the element as this was already done
	// we need to validate the key
	key := req.GetPath()[0].GetElem()[0].GetKey()
	if _, ok := key["name"]; !ok {
		return nil, errors.New("Devictype key not provided")
	}
	deviceType := key["name"]

	// check if the device type was registered
	var data []byte
	var err error
	log.Debug("Get Register..", "Data", c.RegisteredDevices)
	if dt, ok := c.RegisteredDevices[nddv1.DeviceType(deviceType)]; !ok {
		// object not found
		r := &nddv1.Register{}
		data, err = json.Marshal(r)
		if err != nil {
			return nil, err
		}
		// set the register path to default with a dummy keyValue
		req.Path[0] = nddv1.RegisterPath
	} else {
		r := &nddv1.Register{
			Subscriptions: dt.Subscriptions,
		}
		data, err = json.Marshal(r)
		if err != nil {
			return nil, err
		}
	}

	updates := make([]*gnmi.Update, 0)
	upd := &gnmi.Update{
		Path: req.GetPath()[0],
		Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: data}},
	}
	updates = append(updates, upd)

	log.Debug("Registration Read reply...", "Reply", string(data))
	return getResponse(req, req.GetExtension(), updates), nil
}
