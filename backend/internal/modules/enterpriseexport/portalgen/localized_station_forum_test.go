package portalgen

import (
	"strings"
	"testing"
)

func TestLocalizedStationPagesPreserveSurveyHistoryAndSourceFacts(t *testing.T) {
	out := generateSEOPortal(t, true)
	id := seoID(1)
	for _, check := range []struct {
		locale, prefix, registration, history, linked, pricePath string
	}{
		{"en", "en/stations/", "Registration and address", "Price history by survey", "Linked official records", "/en/fuel-prices/mt/cuiaba/"},
		{"es", "es/estaciones/", "Registro y dirección", "Historial de precios por encuesta", "Registros oficiales vinculados", "/es/precios/mt/cuiaba/"},
	} {
		t.Run(check.locale, func(t *testing.T) {
			path := check.prefix + id + "/"
			html := readPortal(t, out, path+"index.html")
			for _, want := range []string{
				`<html lang="` + check.locale + `">`, `rel="canonical" href="` + seoSite + "/" + path + `"`,
				`hreflang="pt-BR" href="` + seoSite + "/postos/" + id + `/"`, `content="` + robotsIndexable + `"`,
				`"@type":"GasStation"`, check.registration, check.history, check.linked,
				`href="` + check.pricePath + `"`, `Auto Posto Cuiabá Ltda`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("%s is absent from %s", want, path)
				}
			}
			for _, unwanted := range []string{"Histórico por pesquisa", "Identidade e endereço", "À vista", "latitude", "longitude"} {
				if strings.Contains(html, unwanted) {
					t.Errorf("%s is present in %s", unwanted, path)
				}
			}
		})
	}
}

func TestLocalizedForumUsesReviewedTaxonomyAndMarksOriginalPostBody(t *testing.T) {
	out := generateSEOPortal(t, true)
	for _, check := range []struct{ locale, hub, category, by, original string }{
		{"en", "en/forum/", "Fuels", "By", "This post is displayed in its original Portuguese."},
		{"es", "es/foro/", "Combustibles", "Por", "Esta publicación se muestra en su versión original en portugués."},
	} {
		t.Run(check.locale, func(t *testing.T) {
			index := readPortal(t, out, check.hub+"index.html")
			category := readPortal(t, out, check.hub+"combustiveis/index.html")
			if !strings.Contains(index, `href="/`+check.hub+`combustiveis/">`+check.category+`</a>`) ||
				!strings.Contains(category, "<h1>"+check.category+"</h1>") ||
				!strings.Contains(category, `hreflang="pt-BR" href="`+seoSite+`/forum/combustiveis/"`) {
				t.Fatal("localized hub and category do not link their matching pages")
			}
			post := readPortal(t, out, check.hub+strings.TrimPrefix(forumPost1, "forum/"))
			for _, want := range []string{
				check.by + ` <a href="/` + check.locale + `/`, check.original,
				`<div class="post-body" lang="pt-BR" data-forum-original>`,
				"<p>O etanol compensa quando custa até 70% da gasolina.</p>",
				`<meta property="og:type" content="article">`, `content="noindex,follow"`,
				`"inLanguage":"pt-BR"`,
			} {
				if !strings.Contains(post, want) {
					t.Error("original post page lacks " + want)
				}
			}
			if strings.Contains(post, `<link rel="alternate" hreflang=`) {
				t.Error("the untranslated forum body was advertised as a translated search result")
			}
		})
	}
}
