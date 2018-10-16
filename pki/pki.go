package pki

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"github.com/kubernetes-incubator/kube-aws/fingerprint"
	"io/ioutil"
	"math"
	"math/big"
	"net"
	"time"
)

type PKI struct {
	specs []KeyPairSpec
}

func NewPKI(specs []KeyPairSpec) *PKI {
	return &PKI{
		specs: specs,
	}
}

func (pki *PKI) fileExists(path string) bool {
	return false
}

func (pki *PKI) encrypt(pem []byte) ([]byte, error) {
	return nil, nil
}

func (pki *PKI) write(path string, data []byte) error {
	return ioutil.WriteFile(path, data, 0644)
}

func (pki *PKI) hash(data []byte) []byte {
	return fingerprint.BytesToSha256(data)
}

func (pki *PKI) EnsureKeyPairsCreated(specs []KeyPairSpec) error {
	for _, spec := range pki.specs {
		keypath := spec.KeyPath()
		shapath := spec.KeyPath() + ".fingerprint"
		encpath := spec.EncryptedKeyPath()
		crtpath := spec.CertPath()
		if !pki.fileExists(encpath) && !pki.fileExists(crtpath) {
			keypair, err := pki.GenerateKeyPair(spec)
			if err != nil {
				return err
			}
			keypem := keypair.KeyInPEM()
			crtpem := keypair.CertInPEM()
			keyenc, err := pki.encrypt(keypem)
			if err != nil {
				return err
			}
			if err := pki.write(keypath, keypem); err != nil {
				return err
			}
			if err := pki.write(encpath, keyenc); err != nil {
				return err
			}
			keysha := pki.hash(keypem)
			if err := pki.write(shapath, keysha); err != nil {
				return err
			}
			if err := pki.write(crtpath, crtpem); err != nil {
				return err
			}
		}
	}
	return nil
}

func (pki *PKI) GenerateKeyPair(spec KeyPairSpec) (*KeyPair, error) {
	key, err := NewPrivateKey()
	if err != nil {
		return nil, err
	}

	if spec.Duration <= 0 {
		return nil, errors.New("self-signed CA cert duration must not be negative or zero")
	}

	keyUsage := x509.KeyUsage(0)
	extKeyUsages := []x509.ExtKeyUsage{}
	isCA := false
	basicConstraintsValid := false

	for _, u := range spec.Usages {
		switch u {
		case "ca":
			keyUsage = keyUsage | x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
			isCA = true
			basicConstraintsValid = true
		case "server":
			keyUsage = keyUsage | x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
			extKeyUsages = append(extKeyUsages, x509.ExtKeyUsageServerAuth)
		case "client":
			keyUsage = keyUsage | x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
			extKeyUsages = append(extKeyUsages, x509.ExtKeyUsageClientAuth)
		default:
			return nil, fmt.Errorf("unsupported usage \"%s\". expected any combination of \"ca\", \"server\", \"client\"", u)
		}
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	ips := make([]net.IP, len(spec.IPAddresses))
	for i, ipStr := range spec.IPAddresses {
		ips[i] = net.ParseIP(ipStr)
	}

	tmpl := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   spec.CommonName,
			Organization: []string{spec.Organization},
		},
		NotBefore:             time.Now().UTC(),
		NotAfter:              time.Now().Add(spec.Duration).UTC(),
		KeyUsage:              keyUsage,
		DNSNames:              spec.DNSNames,
		IPAddresses:           ips,
		ExtKeyUsage:           extKeyUsages,
		BasicConstraintsValid: basicConstraintsValid,
		IsCA:                  isCA,
	}

	caCert := &tmpl
	caKey := key

	if spec.Signer != "" {
		// TODO load signer key and cert
		caKey = nil
		caCert = nil
	}

	certAsn1DERData, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certAsn1DERData)
	if err != nil {
		return nil, err
	}

	return &KeyPair{Key: key, Cert: cert}, nil
}
