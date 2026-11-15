package anchorhome

type ServiceGroup struct{ *PublicService }

func NewServiceGroup() ServiceGroup { return ServiceGroup{PublicService: DefaultPublic} }
