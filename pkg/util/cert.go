package util

import (
	"bytes"
	"encoding/json"
	"html/template"
	"path"

	"github.com/cloudflare/cfssl/api/generator"
	"github.com/cloudflare/cfssl/cli/genkey"
	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/universal"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Certificates struct
type Certificates struct {
	Key  []byte
	Csr  []byte
	Cert []byte
}

// CertInput struct
type CertInput struct {
	Name     string
	LongName string
	Fqdn     string
	Prefix   string
}

// CaRootInput struct
type CaRootInput struct {
	Prefix string
	Names  map[string]string // Not used right now
}

var (
	// Log generic logger
	Log = ctrl.Log.WithName("proxy util")
	// DirLabCA directory
	DirLabCA = "/ca"
	// DirLabCAroot directory
	DirLabCAroot = "/ca/root"
)

// GenerateRootCa function
func GenerateRootCa(csrRootJSONTpl *template.Template, input CaRootInput) (*Certificates, error) {
	Log.Info("Creating root CA")
	//create root CA diretcory
	CreateDirectory(DirLabCA, 0755)

	//create root CA root diretcory
	CreateDirectory(DirLabCAroot, 0755)
	var err error
	csrBuff := new(bytes.Buffer)
	err = csrRootJSONTpl.Execute(csrBuff, input)
	if err != nil {
		return nil, err
	}
	req := csr.CertificateRequest{
		KeyRequest: csr.NewKeyRequest(),
	}
	err = json.Unmarshal(csrBuff.Bytes(), &req)
	if err != nil {
		return nil, err
	}
	//
	var key, csrPEM, cert []byte
	cert, csrPEM, key, err = initca.New(&req)
	if err != nil {
		return nil, err
	}
	certs := &Certificates{
		Key:  key,
		Csr:  csrPEM,
		Cert: cert,
	}
	WriteCertFiles(certs, path.Join(DirLabCAroot, "root-ca"))
	return certs, nil
}

// GenerateCert fucntion
func GenerateCert(name, ca, caKey string, csrJSONTpl *template.Template) (*Certificates, error) {
	input := CertInput{
		Name:     name,
		LongName: name,
		Fqdn:     name + ".srlinux.henderiw.be",
		Prefix:   name,
	}
	CreateDirectory(path.Join(DirLabCA, input.Name), 0755)
	var err error
	csrBuff := new(bytes.Buffer)
	err = csrJSONTpl.Execute(csrBuff, input)
	if err != nil {
		return nil, err
	}

	req := &csr.CertificateRequest{
		KeyRequest: csr.NewKeyRequest(),
	}
	err = json.Unmarshal(csrBuff.Bytes(), req)
	if err != nil {
		return nil, err
	}

	var key, csrBytes []byte
	gen := &csr.Generator{Validator: genkey.Validator}
	csrBytes, key, err = gen.ProcessRequest(req)
	if err != nil {
		return nil, err
	}

	policy := &config.Signing{
		Profiles: map[string]*config.SigningProfile{},
		Default:  config.DefaultConfig(),
	}
	root := universal.Root{
		Config: map[string]string{
			"cert-file": ca,
			"key-file":  caKey,
		},
		ForceRemote: false,
	}
	s, err := universal.NewSigner(root, policy)
	if err != nil {
		return nil, err
	}

	var cert []byte
	signReq := signer.SignRequest{
		Request: string(csrBytes),
	}
	cert, err = s.Sign(signReq)
	if err != nil {
		return nil, err
	}
	if len(signReq.Hosts) == 0 && len(req.Hosts) == 0 {
		Log.Info("Sign Message", "msg", generator.CSRNoHostMessage)
	}
	certs := &Certificates{
		Key:  key,
		Csr:  csrBytes,
		Cert: cert,
	}
	//
	WriteCertFiles(certs, path.Join(DirLabCA, input.Name, input.Name))
	return certs, nil
}

// WriteCertFiles function
func WriteCertFiles(certs *Certificates, filesPrefix string) {
	CreateFile(filesPrefix+".pem", string(certs.Cert))
	CreateFile(filesPrefix+"-key.pem", string(certs.Key))
	CreateFile(filesPrefix+".csr", string(certs.Csr))
}
