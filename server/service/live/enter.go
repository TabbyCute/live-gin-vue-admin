package live

type ServiceGroup struct {
	PresenceService *PresenceService
	AccountService
	AnchorService
	CategoryService
	RoomService
}
