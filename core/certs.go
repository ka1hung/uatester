package core

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"time"
)

// Cert describes a client certificate.
type Cert struct {
	Subject    string    `json:"subject"`
	URI        string    `json:"uri"`
	Thumbprint string    `json:"thumbprint"`
	NotAfter   time.Time `json:"notAfter"`
}

func describe(c *x509.Certificate) Cert {
	sum := sha1.Sum(c.Raw) // OPC UA identifies certificates by their SHA-1 thumbprint
	d := Cert{Subject: c.Subject.CommonName, Thumbprint: hex.EncodeToString(sum[:]), NotAfter: c.NotAfter}
	if len(c.URIs) > 0 {
		d.URI = c.URIs[0].String()
	}
	return d
}

// Rejected lists client certificates the server has rejected because
// nobody trusted them yet.
func (s *Server) Rejected() ([]Cert, error) {
	certs, err := s.store.Rejected()
	if err != nil {
		return nil, err
	}
	out := make([]Cert, 0, len(certs))
	for _, c := range certs {
		out = append(out, describe(c))
	}
	return out, nil
}

// Trust trusts a rejected client certificate, so that client can connect
// with security from now on.
func (s *Server) Trust(thumbprint string) error {
	certs, err := s.store.Rejected()
	if err != nil {
		return err
	}
	for _, c := range certs {
		if describe(c).Thumbprint == thumbprint {
			if err := s.store.Trust(c); err != nil {
				return err
			}
			s.log.Info("trusted client certificate", "subject", c.Subject.CommonName)
			return nil
		}
	}
	return fmt.Errorf("no rejected certificate %s", thumbprint)
}
