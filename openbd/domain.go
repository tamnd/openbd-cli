package openbd

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the openbd driver.
type Domain struct{}

// Info describes the scheme, hostnames, and binary identity.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "openbd",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "openbd",
			Short:  "A command line for OpenBD, the Japanese book database.",
			Long: `A command line for OpenBD (api.openbd.jp), the Japanese book database
with 1.9M ISBNs.

openbd reads book metadata over HTTPS, shapes it into clean records, and
prints output that pipes into the rest of your tools. No API key required.`,
			Site: "openbd.jp",
			Repo: "https://github.com/tamnd/openbd-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "get", Group: "read", Single: true,
		Summary: "Get book details by ISBN (comma-separated for batch)",
		Args:    []kit.Arg{{Name: "isbn", Help: "ISBN or comma-separated ISBNs"}}}, getBook)

	kit.Handle(app, kit.OpMeta{Name: "coverage", Group: "read", Single: true,
		Summary: "Show count of ISBNs covered by OpenBD"}, getCoverage)
}

// newClient builds the Client from kit config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- input structs ---

type getBookInput struct {
	ISBN   string  `kit:"arg" help:"ISBN or comma-separated ISBNs"`
	Client *Client `kit:"inject"`
}

type getCoverageInput struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getBook(ctx context.Context, in getBookInput, emit func(*Book) error) error {
	// Split by comma and trim whitespace; rejoin for the API call.
	parts := strings.Split(in.ISBN, ",")
	var cleaned []string
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) == 0 {
		return errs.Usage("isbn must not be empty")
	}
	books, err := in.Client.GetBooks(ctx, strings.Join(cleaned, ","))
	if err != nil {
		return err
	}
	for i := range books {
		if err := emit(&books[i]); err != nil {
			return err
		}
	}
	return nil
}

func getCoverage(ctx context.Context, in getCoverageInput, emit func(*Coverage) error) error {
	cov, err := in.Client.GetCoverage(ctx)
	if err != nil {
		return err
	}
	return emit(cov)
}

// Classify turns any input into (type, id). Everything is a book.
func (Domain) Classify(input string) (string, string, error) {
	return "book", strings.TrimSpace(input), nil
}

// Locate returns the live https URL for a (type, id).
func (Domain) Locate(t, id string) (string, error) {
	switch t {
	case "book":
		return fmt.Sprintf("https://openbd.jp/p/%s", id), nil
	default:
		return "", errs.Usage("openbd has no resource type %q", t)
	}
}
