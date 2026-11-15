package payout

import (
	"crypto/subtle"
	"gorm.io/gorm"
	model "tb_live_module/model/payout"
)

// BankReconciliationAuthorize deliberately reuses the live finance permission
// check, including the existing ID=1 exception. It grants no payout capability.
func (s *Service) BankReconciliationAuthorize(tx *gorm.DB, actor Actor, action, method string) error {
	switch action {
	case "import", "run", "review", "list", "detail":
	default:
		return ErrOrderForbidden
	}
	return s.authorizePayout(tx, actor, "bank-reconciliation/"+action, method)
}

func (s *Service) BankReconciliationDigest(domain string, value any) (string, error) {
	if s == nil || s.Crypto == nil {
		return "", ErrOrderUnavailable
	}
	return s.Crypto.digest("payout.bank-reconciliation.v1."+domain, value), nil
}

// BankStatementIdentity checks the complete source account against its encrypted
// immutable binding. Neither the account nor the raw reference leaves this call.
func (s *Service) BankStatementIdentity(account model.ChannelAccount, sourceAccount, reference string) (string, string, error) {
	if s == nil || s.Crypto == nil {
		return "", "", ErrOrderUnavailable
	}
	plain, err := s.openCompany(account)
	if err != nil {
		return "", "", err
	}
	if account.ChannelCode != "manual_bank" || subtle.ConstantTimeCompare([]byte(plain), []byte(sourceAccount)) != 1 {
		return "", "", ErrOrderInvalid
	}
	normalized, err := normalizeReference(reference)
	if err != nil {
		return "", "", err
	}
	return s.Crypto.digest("payout.payment.reference.v1", []string{account.ChannelAccountNo, normalized}), "****" + normalized[len(normalized)-4:], nil
}

func (s *Service) BankRecipientFingerprint(account string) (string, error) {
	if !cardRE.MatchString(account) {
		return "", ErrOrderInvalid
	}
	return s.BankReconciliationDigest("recipient", account)
}
func (s *Service) BankOrderRecipientFingerprint(order model.Order) (string, error) {
	v, err := s.openOrder(order)
	if err != nil {
		return "", err
	}
	return s.BankRecipientFingerprint(v.Account)
}
