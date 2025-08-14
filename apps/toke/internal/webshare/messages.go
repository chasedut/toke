package webshare

// BuddyEventMsg is sent when buddy-related events occur
type BuddyEventMsg struct {
	Type    string       `json:"type"`    // "joined", "left", "message"
	Buddy   *Buddy       `json:"buddy,omitempty"`
	Message BuddyMessage `json:"message,omitempty"`
}

// SetBuddyInfoMsg updates the buddy info in the UI
type SetBuddyInfoMsg struct {
	Count  int
	Names  []string
}