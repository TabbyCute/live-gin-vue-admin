package wallet

var Default = &Service{}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Default} }
