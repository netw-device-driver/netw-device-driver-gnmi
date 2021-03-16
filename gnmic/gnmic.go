package gnmic

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/gnxi/utils/xpath"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const (
	defaultEncoding = "JSON_IETF"
	defaultTimeout  = 30 * time.Second
	maxMsgSize      = 512 * 1024 * 1024
)

// GnmiClient holds the state of the GNMI configuration
type GnmiClient struct {
	Username   string
	Password   string
	Proxy      bool
	NoTLS      bool
	TLSCA      string
	TLSCert    string
	TLSKey     string
	SkipVerify bool
	Insecure   bool
	Encoding   string
	Timeout    time.Duration
	Target     string
	MaxMsgSize int
	Client     gnmi.GNMIClient
}

// NewGnmiClient return gnmi client
func NewGnmiClient() *GnmiClient {
	return new(GnmiClient)
}

// Initialize the gnmic client
func (g *GnmiClient) Initialize() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := createDialOpts()
	if err := g.CreateGNMIClient(ctx, opts...); err != nil {
		return err
	}
	return nil
}

func createDialOpts() []grpc.DialOption {
	opts := []grpc.DialOption{}
	opts = append(opts, grpc.WithBlock())
	opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)))
	opts = append(opts, grpc.WithNoProxy())
	return opts
}

// CreateGNMIClient create gnmi client
func (g *GnmiClient) CreateGNMIClient(ctx context.Context, opts ...grpc.DialOption) error {
	if opts == nil {
		opts = []grpc.DialOption{}
	}
	if g.Insecure {
		opts = append(opts, grpc.WithInsecure())
	} else {
		tlsConfig, err := g.newTLS()
		if err != nil {
			return err
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	conn, err := grpc.DialContext(timeoutCtx, g.Target, opts...)
	if err != nil {
		return err
	}
	g.Client = gnmi.NewGNMIClient(conn)
	return nil
}

// newTLS sets up a new TLS profile
func (g *GnmiClient) newTLS() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		Renegotiation:      tls.RenegotiateNever,
		InsecureSkipVerify: g.SkipVerify,
	}
	err := g.loadCerts(tlsConfig)
	if err != nil {
		return nil, err
	}
	return tlsConfig, nil
}

func (g *GnmiClient) loadCerts(tlscfg *tls.Config) error {
	/*
		if *c.TLSCert != "" && *c.TLSKey != "" {
			certificate, err := tls.LoadX509KeyPair(*c.TLSCert, *c.TLSKey)
			if err != nil {
				return err
			}
			tlscfg.Certificates = []tls.Certificate{certificate}
			tlscfg.BuildNameToCertificate()
		}
		if c.TLSCA != nil && *c.TLSCA != "" {
			certPool := x509.NewCertPool()
			caFile, err := ioutil.ReadFile(*c.TLSCA)
			if err != nil {
				return err
			}
			if ok := certPool.AppendCertsFromPEM(caFile); !ok {
				return errors.New("failed to append certificate")
			}
			tlscfg.RootCAs = certPool
		}
	*/
	return nil
}

// CreateGetRequest function creates a gnmi get request
func (g *GnmiClient) CreateGetRequest(path *string, dataType string) (*gnmi.GetRequest, error) {
	encodingVal, ok := gnmi.Encoding_value[strings.Replace(strings.ToUpper(g.Encoding), "-", "_", -1)]
	if !ok {
		return nil, fmt.Errorf("invalid encoding type '%s'", g.Encoding)
	}
	dti, ok := gnmi.GetRequest_DataType_value[strings.ToUpper(dataType)]
	if !ok {
		return nil, fmt.Errorf("unknown data type %s", dataType)
	}
	req := &gnmi.GetRequest{
		UseModels: make([]*gnmi.ModelData, 0),
		Path:      make([]*gnmi.Path, 0),
		Encoding:  gnmi.Encoding(encodingVal),
		Type:      gnmi.GetRequest_DataType(dti),
	}
	prefix := ""
	if prefix != "" {
		gnmiPrefix, err := ParsePath(prefix)
		if err != nil {
			return nil, fmt.Errorf("prefix parse error: %v", err)
		}
		req.Prefix = gnmiPrefix
	}

	gnmiPath, err := ParsePath(strings.TrimSpace(*path))
	if err != nil {
		return nil, fmt.Errorf("path parse error: %v", err)
	}
	req.Path = append(req.Path, gnmiPath)
	return req, nil
}

// CreateSetRequest function creates a gnmi set request
func (g *GnmiClient) CreateSetRequest(path *string, updateBytes []byte) (*gnmi.SetRequest, error) {
	/*
		updateBytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal error: %v", err)
		}
	*/
	value := new(gnmi.TypedValue)
	value.Value = &gnmi.TypedValue_JsonIetfVal{
		JsonIetfVal: bytes.Trim(updateBytes, " \r\n\t"),
	}

	gnmiPrefix, err := CreatePrefix("", "")
	if err != nil {
		return nil, fmt.Errorf("prefix parse error: %v", err)
	}

	gnmiPath, err := ParsePath(strings.TrimSpace(*path))
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
func (g *GnmiClient) CreateDeleteRequest(path *string) (*gnmi.SetRequest, error) {
	gnmiPrefix, err := CreatePrefix("", "")
	if err != nil {
		return nil, fmt.Errorf("prefix parse error: %v", err)
	}

	gnmiPath, err := ParsePath(strings.TrimSpace(*path))
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

// HandleGetResponse handes the response
func (g *GnmiClient) HandleGetResponse(response *gnmi.GetResponse) ([]update, error) {
	for _, notif := range response.GetNotification() {

		updates := make([]update, 0, len(notif.GetUpdate()))

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
			value, err := getValue(upd.GetVal())
			if err != nil {
				return nil, err
			}
			updates = append(updates,
				update{
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

type update struct {
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
		sb.WriteString(pe.GetName())
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

func getValue(updValue *gnmi.TypedValue) (interface{}, error) {
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

// CreatePrefix function
func CreatePrefix(prefix, target string) (*gnmi.Path, error) {
	if len(prefix)+len(target) == 0 {
		return nil, nil
	}
	p, err := ParsePath(prefix)
	if err != nil {
		return nil, err
	}
	if target != "" {
		p.Target = target
	}
	return p, nil
}

// ParsePath creates a gnmi.Path out of a p string, check if the first element is prefixed by an origin,
// removes it from the xpath and adds it to the returned gnmiPath
func ParsePath(p string) (*gnmi.Path, error) {
	var origin string
	elems := strings.Split(p, "/")
	if len(elems) > 0 {
		f := strings.Split(elems[0], ":")
		if len(f) > 1 {
			origin = f[0]
			elems[0] = strings.Join(f[1:], ":")
		}
	}
	gnmiPath, err := xpath.ToGNMIPath(strings.Join(elems, "/"))
	if err != nil {
		return nil, err
	}
	gnmiPath.Origin = origin
	return gnmiPath, nil
}

// Capabilities sends a gnmi.CapabilitiesRequest to the target and returns a gnmi.CapabilitiesResponse and an error
func (g *GnmiClient) Capabilities(ctx context.Context, ext ...*gnmi_ext.Extension) (*gnmi.CapabilityResponse, error) {
	// TODO get credentials

	ctx = metadata.AppendToOutgoingContext(ctx, "username", g.Username, "password", g.Password)
	response, err := g.Client.Capabilities(ctx, &gnmi.CapabilityRequest{Extension: ext})
	if err != nil {
		return nil, fmt.Errorf("failed sending capabilities request: %v", err)
	}
	return response, nil
}

// Get sends a gnmi.GetRequest to the target *t and returns a gnmi.GetResponse and an error
func (g *GnmiClient) Get(ctx context.Context, req *gnmi.GetRequest) (response *gnmi.GetResponse, err error) {
	nctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	nctx = metadata.AppendToOutgoingContext(nctx, "username", g.Username, "password", g.Password)
	response, err = g.Client.Get(nctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed sending GetRequest to '%s': %v", g.Target, err)
	}
	return response, nil
}

// Set sends a gnmi.SetRequest to the target *t and returns a gnmi.SetResponse and an error
func (g *GnmiClient) Set(ctx context.Context, req *gnmi.SetRequest) (response *gnmi.SetResponse, err error) {
	nctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	nctx = metadata.AppendToOutgoingContext(nctx, "username", g.Username, "password", g.Password)
	response, err = g.Client.Set(nctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed sending SetRequest to '%s': %v", g.Target, err)
	}
	/*
		for i := 0; i < 5; i++ {
			response, err = t.Client.Set(nctx, req)
			if err != nil {
				log.Errorf("Try %d, Set failed: %v", i, err)
				rand.Seed(time.Now().UnixNano())
				r := rand.Intn(1000)
				time.Sleep(time.Duration(r) * time.Millisecond)
				if i == 5 {
					return nil, fmt.Errorf("failed sending SetRequest 5 times to '%s': %v", *t.Config.Target, err)
				}
			} else {
				break
			}
		}
	*/
	return response, nil
}
