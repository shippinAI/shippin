package tenantdb

// Template is a starter shape for an empty database. Statements are fixed;
// they are still classified and run as the tenant role.
type Template struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Hint        string   `json:"hint"`
	Tables      []string `json:"tables"`
	Statements  []string `json:"statements"`
}

// Templates is the empty-state gallery.
func Templates() []Template {
	return []Template{todoTemplate, crmTemplate, blogTemplate}
}

// TemplateByID returns one template.
func TemplateByID(id string) (Template, bool) {
	for _, t := range Templates() {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

var todoTemplate = Template{
	ID: "todo", Title: "Todo list",
	Description: "A single list of tasks with a done checkbox.",
	Hint:        "Good when you just want to save something and see it in the grid.",
	Tables:      []string{"todos"},
	Statements: []string{`CREATE TABLE IF NOT EXISTS todos (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title text NOT NULL,
  done boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
)`},
}

var crmTemplate = Template{
	ID: "crm", Title: "CRM",
	Description: "Companies, the people who work there, and deals.",
	Hint:        "Three tables linked together, the way a small sales list usually starts.",
	Tables:      []string{"companies", "contacts", "deals"},
	Statements: []string{
		`CREATE TABLE IF NOT EXISTS companies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  website text,
  created_at timestamptz NOT NULL DEFAULT now()
)`,
		`CREATE TABLE IF NOT EXISTS contacts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id uuid REFERENCES companies (id) ON DELETE SET NULL,
  name text NOT NULL,
  email text,
  created_at timestamptz NOT NULL DEFAULT now()
)`,
		`CREATE TABLE IF NOT EXISTS deals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  contact_id uuid REFERENCES contacts (id) ON DELETE SET NULL,
  title text NOT NULL,
  amount numeric,
  stage text NOT NULL DEFAULT 'new',
  created_at timestamptz NOT NULL DEFAULT now()
)`,
	},
}

var blogTemplate = Template{
	ID: "blog", Title: "Blog",
	Description: "Posts and the comments under them.",
	Hint:        "A title, a slug, a body, and a published switch.",
	Tables:      []string{"posts", "comments"},
	Statements: []string{
		`CREATE TABLE IF NOT EXISTS posts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title text NOT NULL,
  slug text NOT NULL UNIQUE,
  body text NOT NULL DEFAULT '',
  published boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
)`,
		`CREATE TABLE IF NOT EXISTS comments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  post_id uuid NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
  author text NOT NULL,
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
)`,
	},
}
