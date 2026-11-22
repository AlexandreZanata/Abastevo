package portalgen

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:generate cp ../../../../../contracts/i18n/locales.json assets/locales.json
//go:embed assets/locales.json assets/messages/*.json
var localizationAssets embed.FS

const defaultPortalLocale = "pt-BR"

type portalLocale struct {
	Code       string
	Prefix     string
	HTMLLang   string
	Hreflang   string
	OGLocale   string
	Decimal    string
	DateLayout string
	Enabled    bool
	Reviewers  []string
	Segments   map[string]string
	FuelSlugs  map[string]string
	catalog    portalCatalog
	numbers    *message.Printer
}

type portalCatalog struct {
	TranslationStatus string
	ReviewedBy        string
	ReviewedAt        string
	Messages          map[string]string
}

var (
	routeSlug    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	localePrefix = regexp.MustCompile(`^/[a-z]{2}$`)
	placeholder  = regexp.MustCompile(`\{([a-z]+)\}`)
)

func readLocaleJSON(assets fs.FS, path string, target any) error {
	raw, err := fs.ReadFile(assets, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("portalgen: invalid %s: %w", path, err)
	}
	return nil
}

func loadPortalLocales(assets fs.FS) (map[string]portalLocale, error) {
	var registry struct {
		Version int
		Default string
		Locales []portalLocale
	}
	if err := readLocaleJSON(assets, "assets/locales.json", &registry); err != nil {
		return nil, err
	}
	if registry.Version != 1 || registry.Default != defaultPortalLocale {
		return nil, fmt.Errorf("portalgen: unsupported locale registry")
	}
	locales := make(map[string]portalLocale, len(registry.Locales))
	prefixes := map[string]bool{}
	for _, locale := range registry.Locales {
		tag, err := language.Parse(locale.Code)
		if err != nil || locale.HTMLLang == "" || locale.Hreflang == "" || locale.OGLocale == "" ||
			(locale.Decimal != "." && locale.Decimal != ",") || locale.DateLayout == "" ||
			(locale.Code == defaultPortalLocale && (locale.Prefix != "" || !locale.Enabled)) ||
			(locale.Code != defaultPortalLocale && !localePrefix.MatchString(locale.Prefix)) ||
			prefixes[locale.Prefix] || locales[locale.Code].Code != "" {
			return nil, fmt.Errorf("portalgen: invalid locale %q", locale.Code)
		}
		for _, segment := range []string{"guias", "ferramentas", "precos", "metodologia", "dados", "curiosidades",
			"consultar", "postos", "forum", "fiscalizacao", "fiscal-pagina", "fiscal-registro", "como-usar",
			"privacidade", "excluir-conta", "termos"} {
			if !routeSlug.MatchString(locale.Segments[segment]) {
				return nil, fmt.Errorf("portalgen: invalid route segment %q for %s", segment, locale.Code)
			}
		}
		slugs := map[string]bool{}
		for _, fuel := range fuelVocabulary {
			slug := locale.FuelSlugs[fuel.Slug]
			if !routeSlug.MatchString(slug) || len(slug) == 2 || slugs[slug] {
				return nil, fmt.Errorf("portalgen: invalid fuel route %q for %s", slug, locale.Code)
			}
			slugs[slug] = true
		}
		if err := readLocaleJSON(assets, "assets/messages/"+locale.Code+".json", &locale.catalog); err != nil && !isMissingOptionalCatalog(err, locale.Code) {
			return nil, err
		}
		// Spanish uses Latin American vocabulary and a Brazilian currency
		// display convention, including digit grouping for figures below 10,000.
		if locale.Code == "es" {
			tag = language.BrazilianPortuguese
		}
		locale.numbers = message.NewPrinter(tag)
		locales[locale.Code] = locale
		prefixes[locale.Prefix] = true
	}
	source, ok := locales[defaultPortalLocale]
	if !ok || !source.reviewed() {
		return nil, fmt.Errorf("portalgen: Portuguese catalog must be reviewed")
	}
	for _, locale := range locales {
		if locale.Code != defaultPortalLocale && locale.reviewed() {
			if err := validatePortalCatalog(source.catalog, locale.catalog); err != nil {
				return nil, fmt.Errorf("portalgen: %s: %w", locale.Code, err)
			}
		}
	}
	return locales, nil
}

func isMissingOptionalCatalog(err error, locale string) bool {
	return locale != defaultPortalLocale && errors.Is(err, fs.ErrNotExist)
}

func validatePortalCatalog(source, translated portalCatalog) error {
	if len(source.Messages) != len(translated.Messages) {
		return fmt.Errorf("catalog key counts differ")
	}
	for _, key := range slices.Sorted(maps.Keys(source.Messages)) {
		original, translation := source.Messages[key], translated.Messages[key]
		if original == "" || translation == "" {
			return fmt.Errorf("catalog message %q is missing", key)
		}
		left, right := placeholder.FindAllString(original, -1), placeholder.FindAllString(translation, -1)
		slices.Sort(left)
		slices.Sort(right)
		if !slices.Equal(left, right) {
			return fmt.Errorf("catalog placeholders differ for %q", key)
		}
	}
	return nil
}

func (l portalLocale) reviewed() bool {
	_, dateErr := parseDay(l.catalog.ReviewedAt)
	return l.catalog.TranslationStatus == "reviewed" && dateErr == nil &&
		l.catalog.ReviewedBy != "" && slices.Contains(l.Reviewers, l.catalog.ReviewedBy)
}

func (l portalLocale) route(kind, slug, area, city string) string {
	base := l.Prefix + "/"
	switch kind {
	case "guide":
		return base + l.Segments["guias"] + "/" + slug + "/"
	case "tool":
		return base + l.Segments["ferramentas"] + "/" + slug + "/"
	case "page":
		return base + slug + "/"
	case "methodology":
		return base + l.Segments["metodologia"] + "/"
	case "data":
		return base + l.Segments["dados"] + "/"
	case "insights":
		return base + l.Segments["curiosidades"] + "/"
	case "search":
		return base + l.Segments["consultar"] + "/"
	case "stations", "forum":
		segment := l.Segments["forum"]
		if kind == "stations" {
			segment = l.Segments["postos"]
		}
		if slug != "" {
			return base + segment + "/" + slug + "/"
		}
		return base + segment + "/"
	case "enforcement", "enforcementPage", "enforcementRecord":
		base += l.Segments["fiscalizacao"] + "/"
		switch kind {
		case "enforcementPage":
			return base + l.Segments["fiscal-pagina"] + "/" + slug + "/"
		case "enforcementRecord":
			return base + l.Segments["fiscal-registro"] + "/" + slug + "/"
		}
		return base
	case "prices":
		parts := []string{l.Segments["precos"]}
		if slug != "" {
			if fuel := l.FuelSlugs[slug]; fuel != "" {
				parts = append(parts, fuel)
			} else {
				return ""
			}
		}
		for _, part := range []string{area, city} {
			if part != "" {
				parts = append(parts, part)
			}
		}
		return base + strings.Join(parts, "/") + "/"
	}
	return ""
}

func (l portalLocale) text(key string, fields ...string) string {
	pattern, exists := l.catalog.Messages[key]
	if !exists {
		panic("portalgen: unknown message " + key)
	}
	return placeholder.ReplaceAllStringFunc(pattern, func(field string) string {
		for i := 0; i+1 < len(fields); i += 2 {
			if fields[i] == field[1:len(field)-1] {
				return fields[i+1]
			}
		}
		panic("portalgen: missing field " + field + " for " + key)
	})
}

func (l portalLocale) date(iso string) string {
	if day, err := parseDay(iso); err == nil {
		return day.Format(l.DateLayout)
	}
	return l.text("unknown")
}

func (l portalLocale) period(start, end string) string {
	if start == "" || end == "" {
		return l.text("period.unknown")
	}
	return l.text("period.range", "start", l.date(start), "end", l.date(end))
}

func (l portalLocale) money(milli int64) string {
	sign := ""
	if milli < 0 {
		sign, milli = "-", -milli
	}
	// Preserve a third decimal when the survey quotes a tenth of a cent.
	fraction := fmt.Sprintf("%03d", milli%1000)
	if fraction[2] == '0' {
		fraction = fraction[:2]
	}
	return "R$ " + sign + l.numbers.Sprintf("%d", milli/1000) + l.Decimal + fraction
}

func (l portalLocale) integer(value int) string { return l.numbers.Sprintf("%d", value) }

func (l portalLocale) unit(unit string) string { return l.text("unit." + unit) }

func (l portalLocale) fuel(code string) string { return l.text("fuel." + code) }

func (l portalLocale) priceDescription(key string, fields ...string) string {
	description := l.text(key, fields...)
	if len([]rune(description)) < descriptionMin {
		description += " " + l.text("description.source")
	}
	return truncateWords(description, descriptionMax)
}
