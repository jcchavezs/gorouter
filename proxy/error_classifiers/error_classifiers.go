package error_classifiers

import (
	"crypto/tls"
	"crypto/x509"
	"net"
)

//go:generate counterfeiter -o fakes/fake_classifier.go --fake-name Classifier . Classifier
type Classifier interface {
	Classify(err error) bool
}

type ClassifierFunc func(err error) bool

func (f ClassifierFunc) Classify(err error) bool { return f(err) }

var AttemptedTLSWithNonTLSBackend = ClassifierFunc(func(err error) bool {
	switch err.(type) {
	case tls.RecordHeaderError, *tls.RecordHeaderError:
		return true
	default:
		return false
	}
})

var Dial = ClassifierFunc(func(err error) bool {
	ne, ok := err.(*net.OpError)
	return ok && ne.Op == "dial"
})

var ConnectionResetOnRead = ClassifierFunc(func(err error) bool {
	ne, ok := err.(*net.OpError)
	return ok && ne.Op == "read" && ne.Err.Error() == "read: connection reset by peer"
})

var RemoteFailedCertCheck = ClassifierFunc(func(err error) bool {
	ne, ok := err.(*net.OpError)
	return ok && ne.Op == "remote error" && ne.Err.Error() == "tls: bad certificate"
})

var RemoteHandshakeFailure = ClassifierFunc(func(err error) bool {
	ne, ok := err.(*net.OpError)
	return ok && ne.Op == "remote error" && ne.Err.Error() == "tls: handshake failure"
})

var HostnameMismatch = ClassifierFunc(func(err error) bool {
	switch err.(type) {
	case x509.HostnameError, *x509.HostnameError:
		return true
	default:
		return false
	}
})

var UntrustedCert = ClassifierFunc(func(err error) bool {
	switch err.(type) {
	case x509.UnknownAuthorityError, *x509.UnknownAuthorityError:
		return true
	default:
		return false
	}
})