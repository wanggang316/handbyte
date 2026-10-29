package scanner

import "time"

// Safety indicates how risky it is to delete an item.
type Safety int

const (
	// SafetyAuto: regenerable cache, no questions asked.
	SafetyAuto Safety = iota
	// SafetyReview: likely safe but worth a glance (project build artifacts).
	SafetyReview
	// SafetyCaution: user data or live state — needs deliberate confirmation.
	SafetyCaution
)

func (s Safety) String() string {
	switch s {
	case SafetyAuto:
		return "safe"
	case SafetyReview:
		return "review"
	case SafetyCaution:
		return "caution"
	}
	return "?"
}

// Category groups items in the UI.
type Category string

const (
	CatSystemCache  Category = "System & App Caches"
	CatDevCache     Category = "Developer Caches"
	CatXcode        Category = "Xcode & iOS"
	CatProjectBuild Category = "Project Build Artifacts"
	CatVMs          Category = "VMs & Containers"
	CatTrash        Category = "Trash & Logs"
	CatOther        Category = "Other Large Directories"
)

// Item is one cleanable target.
type Item struct {
	Name        string
	Owner       string
	Project     string
	Scope       string
	Identity    Identity
	Eligibility Eligibility
	Reason      string
	Activity    Activity
	ScannedAt   time.Time
	Path        string
	Size        int64
	FileCount   int64
	ModTime     time.Time
	Category    Category
	Safety      Safety
	Description string // why it's listed and what cleaning means
	// Source records which preset/pattern produced this item, for grouping.
	Source     string
	ScanStatus string
	ScanError  string
}

// Identity binds a reviewed candidate to a filesystem object.
type Identity struct {
	Device uint64
	Inode  uint64
}

type Eligibility string

const (
	Recommended Eligibility = "recommended"
	Review      Eligibility = "review"
	Protected   Eligibility = "protected"
)

type Activity string

const (
	Idle    Activity = "idle"
	Active  Activity = "active"
	Unknown Activity = "unknown"
)

func (it Item) Selectable() bool { return it.Eligibility == Recommended || it.Eligibility == Review }
