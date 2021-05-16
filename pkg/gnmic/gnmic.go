package gnmic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/karimra/gnmic/collector"
	"github.com/openconfig/gnmi/proto/gnmi"
	log "github.com/sirupsen/logrus"
	"github.com/stoewer/go-strcase"
)

const (
	defaultEncoding = "JSON_IETF"
	defaultTimeout  = 30 * time.Second
	maxMsgSize      = 512 * 1024 * 1024
)

func SubName(s *string) *string {
	split := strings.Split(*s, "/")
	subName := "sub"
	for _, n := range split {
		subName += strcase.UpperCamelCase(n)
	}
	return &subName
}

func CreateSubscriptionRequest(subPaths *[]string) (*gnmi.SubscribeRequest, error) {
	// create subscription
	paths := *subPaths

	/*
		sc := &collector.SubscriptionConfig{
			Name:       subName,
			Prefix:     "",
			Target:     "",
			Paths:      paths,
			Mode:       "STREAM",
			StreamMode: "ON_CHANGE",
			Encoding:   "JSON_IETF",
			Qos:        Uint32Ptr(21),
		}
		req, err := sc.CreateSubscribeRequest()
		if err != nil {
			log.WithError(err).Error("subscription creation failed")
		}
	*/

	gnmiPrefix, err := collector.CreatePrefix("", "")
	if err != nil {
		return nil, fmt.Errorf("create prefix failed")
	}
	modeVal, _ := gnmi.SubscriptionList_Mode_value[strings.ToUpper("STREAM")]
	//models := make([]*gnmi.ModelData, 0, len(sc.Models))
	qos := &gnmi.QOSMarking{Marking: 21}

	subscriptions := make([]*gnmi.Subscription, len(paths))
	for i, p := range paths {
		gnmiPath, _ := collector.ParsePath(strings.TrimSpace(p))
		subscriptions[i] = &gnmi.Subscription{Path: gnmiPath}
		switch gnmi.SubscriptionList_Mode(modeVal) {
		case gnmi.SubscriptionList_STREAM:
			mode, _ := gnmi.SubscriptionMode_value[strings.Replace(strings.ToUpper("ON_CHANGE"), "-", "_", -1)]
			subscriptions[i].Mode = gnmi.SubscriptionMode(mode)
			/*
				switch gnmi.SubscriptionMode(mode) {
				case gnmi.SubscriptionMode_ON_CHANGE:
					if sc.HeartbeatInterval != nil {
						subscriptions[i].HeartbeatInterval = uint64(sc.HeartbeatInterval.Nanoseconds())
					}
				case gnmi.SubscriptionMode_SAMPLE, gnmi.SubscriptionMode_TARGET_DEFINED:
					if sc.SampleInterval != nil {
						subscriptions[i].SampleInterval = uint64(sc.SampleInterval.Nanoseconds())
					}
					subscriptions[i].SuppressRedundant = sc.SuppressRedundant
					if subscriptions[i].SuppressRedundant {
						subscriptions[i].HeartbeatInterval = uint64(sc.HeartbeatInterval.Nanoseconds())
					}
				}
			*/
		}
	}

	req := &gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{
				Prefix:       gnmiPrefix,
				Mode:         gnmi.SubscriptionList_Mode(modeVal),
				Encoding:     46, // "JSON_IETF_CONFIG_ONLY"
				Subscription: subscriptions,
				Qos:          qos,
				//UpdatesOnly:  sc.UpdatesOnly,
				//UseModels:    models,
			},
		},
	}
	return req, nil
}

// CreateGetRequest function creates a gnmi get request
func CreateGetRequest(path, dataType, encoding *string) (*gnmi.GetRequest, error) {
	if encoding == nil {
		encoding = StringPtr(defaultEncoding)
	}
	encodingVal, ok := gnmi.Encoding_value[strings.Replace(strings.ToUpper(defaultEncoding), "-", "_", -1)]
	if !ok {
		return nil, fmt.Errorf("invalid encoding type '%s'", *encoding)
	}
	dti, ok := gnmi.GetRequest_DataType_value[strings.ToUpper(*dataType)]
	if !ok {
		return nil, fmt.Errorf("unknown data type %s", *dataType)
	}
	req := &gnmi.GetRequest{
		UseModels: make([]*gnmi.ModelData, 0),
		Path:      make([]*gnmi.Path, 0),
		Encoding:  gnmi.Encoding(encodingVal),
		Type:      gnmi.GetRequest_DataType(dti),
	}
	prefix := ""
	if prefix != "" {
		gnmiPrefix, err := collector.ParsePath(prefix)
		if err != nil {
			return nil, fmt.Errorf("prefix parse error: %v", err)
		}
		req.Prefix = gnmiPrefix
	}

	gnmiPath, err := collector.ParsePath(strings.TrimSpace(*path))
	if err != nil {
		return nil, fmt.Errorf("path parse error: %v", err)
	}
	req.Path = append(req.Path, gnmiPath)
	return req, nil
}

// CreateSetRequest function creates a gnmi set request
func CreateSetRequest(path *string, updateBytes []byte) (*gnmi.SetRequest, error) {
	value := new(gnmi.TypedValue)
	value.Value = &gnmi.TypedValue_JsonIetfVal{
		JsonIetfVal: bytes.Trim(updateBytes, " \r\n\t"),
	}

	gnmiPrefix, err := collector.CreatePrefix("", "")
	if err != nil {
		return nil, fmt.Errorf("prefix parse error: %v", err)
	}

	gnmiPath, err := collector.ParsePath(strings.TrimSpace(*path))
	if err != nil {
		return nil, fmt.Errorf("path parse error: %v", err)
	}

	req := &gnmi.SetRequest{
		Prefix:  gnmiPrefix,
		Delete:  make([]*gnmi.Path, 0, 0),
		Replace: make([]*gnmi.Update, 0),
		Update:  make([]*gnmi.Update, 0),
	}

	req.Update = append(req.Update, &gnmi.Update{
		Path: gnmiPath,
		Val:  value,
	})

	return req, nil
}

// CreateDeleteRequest function
func CreateDeleteRequest(path *string) (*gnmi.SetRequest, error) {
	gnmiPrefix, err := collector.CreatePrefix("", "")
	if err != nil {
		return nil, fmt.Errorf("prefix parse error: %v", err)
	}

	gnmiPath, err := collector.ParsePath(strings.TrimSpace(*path))
	if err != nil {
		return nil, fmt.Errorf("path parse error: %v", err)
	}

	req := &gnmi.SetRequest{
		Prefix:  gnmiPrefix,
		Delete:  make([]*gnmi.Path, 0, 0),
		Replace: make([]*gnmi.Update, 0),
		Update:  make([]*gnmi.Update, 0),
	}

	req.Delete = append(req.Delete, gnmiPath)

	return req, nil
}

// HandleSubscriptionResponse handes the response
func HandleSubscriptionResponse(resp *gnmi.SubscribeResponse) ([]Update, error) {
	updates := make([]Update, 0)
	switch resp.GetResponse().(type) {
	case *gnmi.SubscribeResponse_Update:
		log.Info("SubscribeResponse_Update")

		u := resp.GetUpdate().Update
		for i, upd := range u {
			// Path element processing
			pathElems := make([]string, 0, len(upd.GetPath().GetElem()))
			for _, pElem := range upd.GetPath().GetElem() {
				log.Infof("pElem: %v", pElem)
				pathElems = append(pathElems, pElem.GetName())
			}
			var pathElemSplit []string
			var pathElem string
			if len(pathElems) != 0 {
				if len(pathElems) > 1 {
					pathElemSplit = strings.Split(pathElems[len(pathElems)-1], ":")
				} else {
					pathElemSplit = strings.Split(pathElems[0], ":")
				}

				if len(pathElemSplit) > 1 {
					pathElem = pathElemSplit[len(pathElemSplit)-1]
				} else {
					pathElem = pathElemSplit[0]
				}
			} else {
				pathElem = ""
			}

			// Value processing
			value, err := GetValue(upd.GetVal())
			if err != nil {
				return nil, err
			}
			updates = append(updates,
				Update{
					Path:   gnmiPathToXPath(upd.GetPath()),
					Values: make(map[string]interface{}),
				})
			updates[i].Values[pathElem] = value
		}
	case *gnmi.SubscribeResponse_SyncResponse:
		log.Info("SubscribeResponse_SyncResponse")
	}

	for i, u := range updates {
		log.Infof("Path: %d, %s", i, u.Path)
		log.Infof("Value: %v", u.Values)
	}

	return updates, nil
}

// HandleGetResponse handes the response
func HandleGetResponse(response *gnmi.GetResponse) ([]Update, error) {
	for _, notif := range response.GetNotification() {

		updates := make([]Update, 0, len(notif.GetUpdate()))

		for i, upd := range notif.GetUpdate() {
			// Path element processing
			pathElems := make([]string, 0, len(upd.GetPath().GetElem()))
			for _, pElem := range upd.GetPath().GetElem() {
				pathElems = append(pathElems, pElem.GetName())
			}
			var pathElemSplit []string
			var pathElem string
			if len(pathElems) != 0 {
				if len(pathElems) > 1 {
					pathElemSplit = strings.Split(pathElems[len(pathElems)-1], ":")
				} else {
					pathElemSplit = strings.Split(pathElems[0], ":")
				}

				if len(pathElemSplit) > 1 {
					pathElem = pathElemSplit[len(pathElemSplit)-1]
				} else {
					pathElem = pathElemSplit[0]
				}
			} else {
				pathElem = ""
			}

			// Value processing
			value, err := GetValue(upd.GetVal())
			if err != nil {
				return nil, err
			}
			updates = append(updates,
				Update{
					Path:   gnmiPathToXPath(upd.GetPath()),
					Values: make(map[string]interface{}),
				})
			updates[i].Values[pathElem] = value

		}
		x, err := json.Marshal(updates)
		if err != nil {
			return nil, nil
		}
		sb := strings.Builder{}
		sb.Write(x)

		return updates, nil
	}
	return nil, nil
}

type Update struct {
	Path   string
	Values map[string]interface{} `json:"values,omitempty"`
}

func gnmiPathToXPath(p *gnmi.Path) string {
	if p == nil {
		return ""
	}
	sb := strings.Builder{}
	if p.Origin != "" {
		sb.WriteString(p.Origin)
		sb.WriteString(":")
	}
	elems := p.GetElem()
	numElems := len(elems)
	for i, pe := range elems {
		var p string
		// remove srl_nokia-interfaces from srl_nokia-interfaces:interface
		pSplit := strings.Split(pe.GetName(), ":")
		if len(pSplit) > 1 {
			p = pSplit[1]
		} else {
			p = pSplit[0]
		}
		sb.WriteString(p)
		for k, v := range pe.GetKey() {
			sb.WriteString("[")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(v)
			sb.WriteString("]")
		}
		if i+1 != numElems {
			sb.WriteString("/")
		}
	}
	return sb.String()
}

func GetValue(updValue *gnmi.TypedValue) (interface{}, error) {
	if updValue == nil {
		return nil, nil
	}
	var value interface{}
	var jsondata []byte
	switch updValue.Value.(type) {
	case *gnmi.TypedValue_AsciiVal:
		value = updValue.GetAsciiVal()
	case *gnmi.TypedValue_BoolVal:
		value = updValue.GetBoolVal()
	case *gnmi.TypedValue_BytesVal:
		value = updValue.GetBytesVal()
	case *gnmi.TypedValue_DecimalVal:
		value = updValue.GetDecimalVal()
	case *gnmi.TypedValue_FloatVal:
		value = updValue.GetFloatVal()
	case *gnmi.TypedValue_IntVal:
		value = updValue.GetIntVal()
	case *gnmi.TypedValue_StringVal:
		value = updValue.GetStringVal()
	case *gnmi.TypedValue_UintVal:
		value = updValue.GetUintVal()
	case *gnmi.TypedValue_JsonIetfVal:
		jsondata = updValue.GetJsonIetfVal()
	case *gnmi.TypedValue_JsonVal:
		jsondata = updValue.GetJsonVal()
	case *gnmi.TypedValue_LeaflistVal:
		value = updValue.GetLeaflistVal()
	case *gnmi.TypedValue_ProtoBytes:
		value = updValue.GetProtoBytes()
	case *gnmi.TypedValue_AnyVal:
		value = updValue.GetAnyVal()
	}
	if value == nil && len(jsondata) != 0 {
		err := json.Unmarshal(jsondata, &value)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}
