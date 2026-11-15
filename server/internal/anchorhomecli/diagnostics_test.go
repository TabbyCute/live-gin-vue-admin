package anchorhomecli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

type unformattedError struct{}

func (unformattedError) Error() string { panic("unknown errors must not be formatted") }

func TestSafeFailureCodeClassifiesWithoutExposingDetails(t *testing.T) {
	const secret = "PRIVATE_DSN_PASSWORD_HOST_SQL"
	wrap := func(err error) error { return fmt.Errorf("%s: %w", secret, err) }
	connection := func(err error) error {
		return &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3306}, Err: &os.SyscallError{Syscall: "connect", Err: err}}
	}
	certificate := &x509.Certificate{DNSNames: []string{secret}}
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"nil", nil, ""},
		{"refused", wrap(connection(syscall.ECONNREFUSED)), "connection_refused"},
		{"network", wrap(connection(syscall.ENETUNREACH)), "network_unreachable"},
		{"host", wrap(connection(syscall.EHOSTUNREACH)), "host_unreachable"},
		{"reset", wrap(connection(syscall.ECONNRESET)), "connection_reset"},
		{"access", wrap(connection(syscall.EACCES)), "access_denied"},
		{"permission", wrap(connection(syscall.EPERM)), "operation_not_permitted"},
		{"aborted", wrap(connection(syscall.ECONNABORTED)), "connection_aborted"},
		{"pipe", wrap(connection(syscall.EPIPE)), "broken_pipe"},
		{"closed", wrap(net.ErrClosed), "connection_closed"},
		{"eof", wrap(io.EOF), "connection_closed"},
		{"unexpected_eof", wrap(io.ErrUnexpectedEOF), "connection_closed"},
		{"dns", wrap(&net.DNSError{Err: secret, Name: secret, Server: secret}), "dns_error"},
		{"dns_timeout", &net.DNSError{Err: secret, Name: secret, IsTimeout: true}, "timeout"},
		{"deadline", wrap(context.DeadlineExceeded), "timeout"},
		{"socket_timeout", wrap(connection(syscall.ETIMEDOUT)), "timeout"},
		{"canceled", wrap(context.Canceled), "canceled"},
		{"mysql", wrap(&driver.MySQLError{Number: 1045, Message: secret}), "mysql_1045"},
		{"tls_authority", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{Cert: certificate}}, "tls_unknown_authority"},
		{"tls_hostname", x509.HostnameError{Certificate: certificate, Host: secret}, "tls_hostname_mismatch"},
		{"tls_invalid", x509.CertificateInvalidError{Cert: certificate, Reason: x509.Expired, Detail: secret}, "tls_certificate_invalid"},
		{"tls_roots", x509.SystemRootsError{Err: errors.New(secret)}, "tls_system_roots"},
		{"tls_verification", &tls.CertificateVerificationError{Err: errors.New(secret)}, "tls_certificate_verification"},
		{"tls_record", tls.RecordHeaderError{Msg: secret}, "tls_record_error"},
		{"tls_record_pointer", &tls.RecordHeaderError{Msg: secret}, "tls_record_error"},
		{"tls_alert", tls.AlertError(40), "tls_alert"},
		{"network_unknown", &net.OpError{Op: secret, Net: secret, Err: errors.New(secret)}, "network_error"},
		{"joined", errors.Join(errors.New(secret), syscall.ECONNRESET), "connection_reset"},
		{"guard", wrap(safeGuardFailure("financial_proof_changed")), "financial_proof_changed"},
		{"unknown_guard", safeGuardFailure(secret), "unknown_error"},
		{"unknown", errors.New(secret), "unknown_error"},
		{"opaque_no_format", unformattedError{}, "unknown_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := safeFailureCode(tc.err)
			require.Equal(t, tc.code, code)
			encoded, err := json.Marshal(Report{Stage: "connection", FailureCode: code})
			require.NoError(t, err)
			for _, private := range []string{secret, "127.0.0.1", "3306", "connect:", "dial tcp"} {
				require.False(t, strings.Contains(string(encoded), private))
			}
		})
	}
}

func TestCleanupClassificationKeepsPrimaryStage(t *testing.T) {
	primary := errors.New("private-primary-error")
	report := Report{Stage: "backup"}
	recordCleanupFailure(&report, &primary, syscall.ECONNRESET)
	require.Equal(t, "backup", report.Stage)
	require.Equal(t, "connection_reset", report.CleanupFailureCode)
	require.Equal(t, "private-primary-error", primary.Error())
}
