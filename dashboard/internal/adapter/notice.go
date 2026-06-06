package adapter

// Notice is a backend message surfaced to the user (errors/warnings/info). It is
// carried on the bus as bus.KindNotice by adapters/managers that want to report
// something to the UI (e.g. a device CRUD failure or a long-running promote). The
// hub also broadcasts notices directly for its own command errors. Living here
// (a leaf package) lets any adapter publish one without importing internal/hub.
type Notice struct {
	Level string // info | warn | error
	Msg   string
}
