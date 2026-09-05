package events

const (
	EventPlayerWeaponFired EventID = iota
	EventHitScanCollision
	EventPlayerInteractWith
	EventPhysicsCollision
	// EventWeaponNoise is a shot being heard. Nothing listens yet: there are no
	// enemies to hear it, and the weapons already compile what they make.
	EventWeaponNoise
)

type EventID uint16

type EventListener func(EventID, Event)

type Event interface{}

var listeners = make(map[EventID][]EventListener)

func Subscribe(eventID EventID, listener EventListener) {
	listeners[eventID] = append(listeners[eventID], listener)
}

func Publish(eventID EventID, event Event) {
	for _, listener := range listeners[eventID] {
		listener(eventID, event)
	}
}
