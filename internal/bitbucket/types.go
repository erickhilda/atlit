package bitbucket

// PullRequest is the subset of the Bitbucket Cloud PR object jt renders.
type PullRequest struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	State       string     `json:"state"`
	Description string     `json:"description"`
	Author      Account    `json:"author"`
	Source      PREndpoint `json:"source"`
	Destination PREndpoint `json:"destination"`
	CreatedOn   string     `json:"created_on"`
	UpdatedOn   string     `json:"updated_on"`
	Links       struct {
		HTML Link `json:"html"`
	} `json:"links"`
}

// Account is a Bitbucket user reference.
type Account struct {
	DisplayName string `json:"display_name"`
}

// PREndpoint is one side (source/destination) of a pull request.
type PREndpoint struct {
	Branch struct {
		Name string `json:"name"`
	} `json:"branch"`
}

// Link is an href wrapper used throughout the Bitbucket API.
type Link struct {
	Href string `json:"href"`
}

// DiffFile is a file reference within a diffstat entry.
type DiffFile struct {
	Path string `json:"path"`
}

// DiffstatEntry summarizes the change to a single file.
type DiffstatEntry struct {
	Status       string    `json:"status"`
	LinesAdded   int       `json:"lines_added"`
	LinesRemoved int       `json:"lines_removed"`
	Old          *DiffFile `json:"old"`
	New          *DiffFile `json:"new"`
}

// Path returns the entry's new path, falling back to the old path (renames,
// deletions) or "?" when neither is present.
func (d DiffstatEntry) Path() string {
	if d.New != nil && d.New.Path != "" {
		return d.New.Path
	}
	if d.Old != nil && d.Old.Path != "" {
		return d.Old.Path
	}
	return "?"
}

// Inline locates an inline comment within a file's diff.
type Inline struct {
	Path string `json:"path"`
	To   *int   `json:"to"`
	From *int   `json:"from"`
}

// Comment is a pull request comment (general or inline).
type Comment struct {
	ID      int `json:"id"`
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	User      Account `json:"user"`
	CreatedOn string  `json:"created_on"`
	Deleted   bool    `json:"deleted"`
	Inline    *Inline `json:"inline"`
	Parent    *struct {
		ID int `json:"id"`
	} `json:"parent"`
}
