package events

const (
    EventWeaponFired EventID = iota
    EventWeaponHitScanHit
    EventWeaponHitScanMiss
    EventPlayerInteractWith
    EventCollision
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
