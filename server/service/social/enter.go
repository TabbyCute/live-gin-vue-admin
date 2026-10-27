package social

var Default = &Service{}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Service: Default} }
