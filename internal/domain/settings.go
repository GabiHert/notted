package domain

// Settings are the person's saved choices.
type Settings struct {
	Profile      string
	CheckoutPath string
	ProjectRoots map[string]string // client key to absolute path; never nil when read
}
