package portalgen

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	jsonLDBlock     = regexp.MustCompile(`<script type="application/ld\+json">(.*?)</script>`)
	visibleCrumbs   = regexp.MustCompile(`<nav class="wrap breadcrumbs"[^>]*>(.*?)</nav>`)
	visibleCrumbTag = regexp.MustCompile(`<a href="([^"]+)">([^<]+)</a>|<span aria-current="page">([^<]+)</span>`)
)

type ldGraph struct {
	Context string           `json:"@context"`
	Graph   []map[string]any `json:"@graph"`
}

func pageGraph(t *testing.T, path, body string) map[string]map[string]any {
	t.Helper()
	blocks := jsonLDBlock.FindAllStringSubmatch(body, -1)
	if len(blocks) != 1 {
		t.Fatalf("%s has %d JSON-LD blocks", path, len(blocks))
	}
	var graph ldGraph
	if err := json.Unmarshal([]byte(blocks[0][1]), &graph); err != nil {
		t.Fatalf("%s JSON-LD does not parse: %v", path, err)
	}
	if graph.Context != "https://schema.org" {
		t.Fatalf("%s JSON-LD context = %q", path, graph.Context)
	}
	byType := map[string]map[string]any{}
	for _, node := range graph.Graph {
		byType[node["@type"].(string)] = node
	}
	return byType
}

func TestStructuredDataDescribesVisibleFacts(t *testing.T) {
	out := generateSEOPortal(t, true)
	pages := portalPages(t, out)
	for path, body := range pages {
		if path == "404.html" {
			if strings.Contains(body, "application/ld+json") {
				t.Error("404 carries structured data")
			}
			continue
		}
		graph := pageGraph(t, path, body)
		crumbs, ok := graph["BreadcrumbList"]
		if !ok {
			t.Fatalf("%s lacks a BreadcrumbList", path)
		}
		items := crumbs["itemListElement"].([]any)
		visible := visibleCrumbs.FindStringSubmatch(body)
		if visible == nil {
			t.Fatalf("%s has no visible breadcrumb", path)
		}
		tags := visibleCrumbTag.FindAllStringSubmatch(visible[1], -1)
		if len(tags) != len(items) {
			t.Fatalf("%s breadcrumb sizes differ: visible %d, JSON-LD %d", path, len(tags), len(items))
		}
		for i, tag := range tags {
			item := items[i].(map[string]any)
			label := html.UnescapeString(tag[2] + tag[3])
			if item["name"] != label {
				t.Errorf("%s crumb %d = %q, visible %q", path, i, item["name"], label)
			}
			if tag[1] != "" && item["item"] != seoSite+tag[1] {
				t.Errorf("%s crumb %d target %q, visible %q", path, i, item["item"], tag[1])
			}
		}
		if first := items[0].(map[string]any); first["name"] != "Início" || first["item"] != seoSite+"/" {
			t.Errorf("%s breadcrumb does not start at Início: %v", path, first)
		}
		for _, forbidden := range []string{"aggregateRating", "offers", "review", "priceSpecification"} {
			if strings.Contains(jsonLDBlock.FindStringSubmatch(body)[1], forbidden) {
				t.Errorf("%s JSON-LD carries %s", path, forbidden)
			}
		}
	}
	station := pageGraph(t, "station", pages["postos/"+seoID(1)+"/index.html"])
	gas := station["GasStation"]
	address, _ := gas["address"].(map[string]any)
	if gas["name"] != "Auto Posto Cuiabá Ltda" || gas["url"] != seoSite+"/postos/"+seoID(1)+"/" || address == nil ||
		address["addressLocality"] != "Cuiabá" || address["addressRegion"] != "MT" || address["streetAddress"] != "AVENIDA CUIABÁ, 10" {
		t.Errorf("GasStation = %v", gas)
	}
	crumbNames := []string{}
	for _, item := range station["BreadcrumbList"]["itemListElement"].([]any) {
		crumbNames = append(crumbNames, item.(map[string]any)["name"].(string))
	}
	if strings.Join(crumbNames, " › ") != "Início › Preços › Mato Grosso › Cuiabá › Auto Posto Cuiabá Ltda" {
		t.Errorf("station breadcrumb = %v", crumbNames)
	}
	for _, path := range []string{"precos/index.html", "consultar/index.html", "precos/mt/index.html", "precos/mt/cuiaba/index.html", "precos/gasolina-comum/index.html"} {
		collection := pageGraph(t, path, pages[path])["CollectionPage"]
		if collection == nil {
			t.Fatalf("%s lacks a CollectionPage", path)
		}
		list := collection["mainEntity"].(map[string]any)
		elements := list["itemListElement"].([]any)
		if list["@type"] != "ItemList" || len(elements) == 0 {
			t.Fatalf("%s ItemList = %v", path, list)
		}
		for _, element := range elements {
			href := strings.TrimPrefix(element.(map[string]any)["url"].(string), seoSite)
			if !strings.Contains(pages[path], `href="`+href+`"`) {
				t.Errorf("%s ItemList entry %s is not a visible link", path, href)
			}
		}
	}
	dataset := pageGraph(t, "dados", pages["dados/index.html"])["Dataset"]
	distribution, _ := dataset["distribution"].([]any)
	license, _ := dataset["license"].(map[string]any)
	if dataset["temporalCoverage"] != "2026-10-11/2026-10-17" || len(distribution) != 4 || license == nil ||
		!strings.Contains(pages["dados/index.html"], license["name"].(string)) || dataset["spatialCoverage"] == nil {
		t.Errorf("Dataset = %v", dataset)
	}
	for _, item := range distribution {
		href := strings.TrimPrefix(item.(map[string]any)["contentUrl"].(string), seoSite)
		if !strings.Contains(pages["dados/index.html"], `href="`+href+`"`) {
			t.Errorf("Dataset download %s is not linked on /dados/", href)
		}
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsInvalidOrCommercialStructuredData(t *testing.T) {
	for name, replacement := range map[string]string{
		"invalid JSON": `"@type":"GasStation",,`,
		"offers":       `"@type":"GasStation","offers":{"price":"6.19"},`,
	} {
		t.Run(name, func(t *testing.T) {
			out := generateSEOPortal(t, true)
			rel := "postos/" + seoID(1) + "/index.html"
			body := readPortal(t, out, rel)
			if !strings.Contains(body, `"@type":"GasStation",`) {
				t.Fatal("GasStation node not found")
			}
			if err := os.WriteFile(filepath.Join(out, rel), []byte(strings.Replace(body, `"@type":"GasStation",`, replacement, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			refreshPortalManifest(t, out)
			if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err == nil || !strings.Contains(err.Error(), "JSON-LD") {
				t.Fatalf("tampered JSON-LD accepted: %v", err)
			}
		})
	}
}
