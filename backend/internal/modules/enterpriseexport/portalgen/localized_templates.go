package portalgen

const localizedTemplates = `
{{define "localized"}}<!DOCTYPE html>
<html lang="{{.Locale.HTMLLang}}">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="robots" content="{{.Robots}}">
  <title>{{.HeadTitle}}</title>
  <meta name="description" content="{{.Description}}">
  {{if .Canonical}}<link rel="canonical" href="{{.Canonical}}">
  <meta property="og:type" content="website">
  <meta property="og:site_name" content="{{msg .Locale "brand"}}">
  <meta property="og:locale" content="{{.Locale.OGLocale}}">
  {{template "alternates" .Links}}
  <meta property="og:url" content="{{.Canonical}}">
  <meta property="og:title" content="{{.HeadTitle}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:image" content="{{.OGImage}}">
  <meta name="twitter:card" content="summary_large_image">
  <meta name="twitter:title" content="{{.HeadTitle}}">
  <meta name="twitter:description" content="{{.Description}}">
  <meta name="twitter:image" content="{{.OGImage}}">{{end}}
  {{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
  <link rel="stylesheet" href="{{.CSSURL}}">
</head>
<body>
  {{$locale := .Locale}}
  <a class="skip" href="#conteudo">{{msg $locale "skip"}}</a>
  <header class="topbar">
    <div class="wrap topbar-inner">
      <a class="brand" href="{{.Home}}"><span class="brand-logo" aria-hidden="true"></span><span class="brand-wordmark">{{msg $locale "brand"}}</span></a>
    </div>
  </header>
  <nav class="portal-nav" aria-label="{{msg $locale "navigation"}}">
    <div class="wrap portal-nav-inner">{{range .Nav}}
      <a href="{{.Href}}"{{if eq .Href $.Route}} aria-current="page"{{end}}>{{.Label}}</a>
    {{end}}</div>
  </nav>
  <nav class="wrap portal-nav-inner" aria-label="{{msg $locale "language"}}">{{template "languageMenuLinks" .Route}}</nav>
  <nav class="wrap breadcrumbs" aria-label="{{msg $locale "breadcrumb"}}">{{range $i, $c := .Breadcrumbs}}{{if $i}} · {{end}}{{if $c.Current}}<span aria-current="page">{{$c.Label}}</span>{{else}}<a href="{{$c.Href}}">{{$c.Label}}</a>{{end}}{{end}}</nav>
  <main id="conteudo" class="wrap">
    <h1>{{.Title}}</h1>
    {{if .ShowPeriod}}<p><strong>{{msg $locale "period.label"}}</strong> · {{.Period}}</p>{{end}}
    <p><strong>{{msg $locale "source"}}</strong> <a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/serie-historica-de-precos-de-combustiveis">{{msg $locale "agency"}}</a></p>
    {{if .IsPrice}}<p>{{msg $locale "price.note"}}</p>
    {{if .NoPrices}}<p class="notice">{{msg $locale "price.empty"}}</p>{{else}}<p>{{msg $locale "price.sample"}}</p>{{end}}{{end}}
    {{range .Tables}}<div class="table-wrap"><table>
      <caption>{{.Title}}</caption>
      <thead><tr>{{range .Headers}}<th scope="col">{{.}}</th>{{end}}</tr></thead>
      <tbody>{{range .Rows}}<tr>{{range .}}<td>{{if .Href}}<a href="{{.Href}}">{{.Text}}</a>{{else}}{{.Text}}{{end}}</td>{{end}}</tr>{{end}}</tbody>
    </table></div>{{end}}
    {{range .Notes}}<section{{if eq .Title (msg $locale "section.notes")}} id="notas"{{end}}>
      <h2>{{.Title}}</h2><p>{{.Body}}</p>
    </section>{{end}}
    {{template "modules" .}}
  </main>
  <footer class="footer">
    <div class="wrap footer-legal">
      <p>{{msg $locale "disclosure"}}</p>
      <p>{{msg $locale "footer"}}</p>
    </div>
  </footer>
</body>
</html>
{{end}}
`
