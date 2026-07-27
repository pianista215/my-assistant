package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

const (
	tlsCertPath = "secrets/tls-cert.pem"
	tlsKeyPath  = "secrets/tls-key.pem"
)

// ensureTLSCert returns the SHA-256 fingerprint (colon-separated uppercase
// hex, matching `openssl x509 -fingerprint -sha256`) and PEM text of the
// TLS certificate at certPath/keyPath. If both files already exist,
// they're reused as-is — regardless of whether this function generated
// them originally or the user dropped in a real CA-issued cert/key pair
// by hand — since regenerating on every restart would change the
// fingerprint and break any ESP32 firmware that has already pinned the
// old one. If only one of the two files exists (a corrupted/partial
// state), both are regenerated. If neither exists, a fresh self-signed
// certificate is generated.
func ensureTLSCert(certPath, keyPath string) (fingerprint string, certPEM string, err error) {
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)

	switch {
	case certErr == nil && keyErr == nil:
		return certFromFile(certPath)
	case certErr == nil || keyErr == nil:
		log.Printf("tls: %s exists but %s does not (or vice versa); regenerating both", certPath, keyPath)
		return generateSelfSignedCert(certPath, keyPath)
	default:
		return generateSelfSignedCert(certPath, keyPath)
	}
}

// certFromFile reads and parses the PEM certificate at path and returns
// its SHA-256 fingerprint and raw PEM text, for ensureTLSCert's reuse
// path.
func certFromFile(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("tls: reading %s: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", "", fmt.Errorf("tls: %s does not contain a PEM certificate", path)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("tls: parsing certificate %s: %w", path, err)
	}

	return fingerprintSHA256(cert.Raw), string(data), nil
}

// generateSelfSignedCert creates a new ECDSA P-256 self-signed
// certificate (10-year validity, SANs from certSANs()), writes the
// cert/key PEM files to certPath/keyPath, and returns its fingerprint
// and PEM text.
func generateSelfSignedCert(certPath, keyPath string) (string, string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("tls: generating key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("tls: generating serial number: %w", err)
	}

	ips, dnsNames := certSANs()
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "my-assistant (self-signed)"},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		IPAddresses:           ips,
		DNSNames:              dnsNames,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("tls: creating certificate: %w", err)
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", fmt.Errorf("tls: marshaling private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("tls: writing %s: %w", keyPath, err)
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEMBytes, 0o644); err != nil {
		return "", "", fmt.Errorf("tls: writing %s: %w", certPath, err)
	}

	return fingerprintSHA256(der), string(certPEMBytes), nil
}

// certSANs returns the IP/DNS Subject Alternative Names for the
// generated certificate: loopback (127.0.0.1, ::1) and "localhost"
// unconditionally, plus whatever non-loopback local IPs
// net.InterfaceAddrs() reports. A failure to list interfaces is logged
// and only loopback SANs are used — not fatal, since SAN/hostname
// matching isn't load-bearing for the intended ESP32 fingerprint-pinning
// use case anyway, it only reduces one class of browser warning for a
// human checking the server via LAN IP.
func certSANs() ([]net.IP, []string) {
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	dnsNames := []string{"localhost"}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("tls: listing interface addresses: %v (certificate SANs limited to loopback)", err)
		return ips, dnsNames
	}
	return append(ips, localNonLoopbackIPs(addrs)...), dnsNames
}

// localNonLoopbackIPs filters addrs (as returned by net.InterfaceAddrs)
// down to the IPs worth adding as SANs: only *net.IPNet entries, with
// loopback and link-local (169.254.0.0/16, fe80::/10 — a link-local
// address needs a zone index to be reachable, which a certificate SAN
// can't carry) addresses dropped. Kept separate from the
// net.InterfaceAddrs() call itself so it's unit-testable with literal
// fixtures instead of depending on the test host's real network
// interfaces.
func localNonLoopbackIPs(addrs []net.Addr) []net.IP {
	var ips []net.IP
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		ips = append(ips, ip)
	}
	return ips
}

// fingerprintSHA256 returns the SHA-256 fingerprint of a DER-encoded
// certificate, formatted as colon-separated uppercase hex (e.g.
// "AB:CD:...:12"), the same format `openssl x509 -fingerprint -sha256`
// prints.
func fingerprintSHA256(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}
