package portalgen

const localizedTemplates = `
{{define "localized"}}<!DOCTYPE html>
<html lang="{{.Locale.HTMLLang}}">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="robots" content="{{.Robots}}">
  <title>{{tabTitle .HeadTitle}}</title>
  {{template "tabIcons" .}}
  <meta name="description" content="{{.Description}}">
  {{if .Canonical}}<link rel="canonical" href="{{.Canonical}}">
  <meta property="og:type" content="{{if .OGType}}{{.OGType}}{{else}}website{{end}}">
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
  {{if .ArticlePublished}}<meta property="article:published_time" content="{{.ArticlePublished}}">{{end}}
  {{if .ArticleModified}}<meta property="article:modified_time" content="{{.ArticleModified}}">{{end}}
  {{if .FeedHref}}<link rel="alternate" type="application/atom+xml" title="{{msg .Locale "title.forum"}}" href="{{.FeedHref}}">{{end}}
  {{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
  <link rel="stylesheet" href="{{.CSSURL}}">
  {{if .ScriptURL}}<script type="module" src="{{.ScriptURL}}" defer></script>{{end}}
</head>
<body>
  {{$locale := .Locale}}
  <a class="skip" href="#conteudo">{{msg $locale "skip"}}</a>
  <header class="topbar">
    <div class="wrap topbar-inner">
      <a class="brand" href="{{.Home}}"><span class="brand-logo" aria-hidden="true"></span><span class="brand-wordmark">{{msg $locale "brand"}}</span></a>
      <nav class="topnav" aria-label="{{msg $locale "navigation"}}">{{template "localizedLinks" .TopNav}}</nav>
      <div class="topbar-actions">
        <a class="topbar-link hide-sm" href="https://github.com/AlexandreZanata/abastevo">GitHub</a>
        <details class="nav-toggle"><summary class="nav-button" aria-label="{{msg $locale "nav.menu"}}"><span aria-hidden="true"></span></summary><nav class="site-nav" aria-label="{{msg $locale "navigation"}}">{{template "localizedLinks" .TopNav}}</nav></details>
      </div>
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
    {{if .Station}}{{template "localizedStation" .}}
    {{else if .Forum}}{{template "localizedForum" .}}
    {{else}}
    <h1>{{.Title}}</h1>
    {{if .Lead}}<p class="lead">{{.Lead}}</p>{{end}}
    {{if .ShowPeriod}}<p><strong>{{msg $locale "period.label"}}</strong> · {{.Period}}</p>{{end}}
    {{if not (or .Enforcement .EnforcementRecord)}}<p><strong>{{msg $locale "source"}}</strong> <a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/serie-historica-de-precos-de-combustiveis">{{msg $locale "agency"}}</a></p>{{end}}
    {{if .Search}}{{template "localizedSearch" .}}{{end}}
    {{if .Enforcement}}{{template "localizedEnforcement" .}}{{end}}
    {{if .EnforcementRecord}}{{template "localizedEnforcementRecord" .}}{{end}}
    {{if .IsPrice}}<p>{{msg $locale "price.note"}}</p>
    {{if .NoPrices}}<p class="notice">{{msg $locale "price.empty"}}</p>{{else}}<p>{{msg $locale "price.sample"}}</p>{{end}}{{end}}
    {{if .Explorer}}{{template "localizedExplorer" .}}{{end}}
    {{if .Reading}}<section id="survey-reading"><h2>{{msg $locale "section.reading"}}</h2><p>{{.Reading}}</p></section>{{end}}
    {{range .Tables}}{{template "localizedTable" .}}{{end}}
    {{if or .Rankings .NoRanking}}<section id="portal-rankings">
      <h2>{{msg $locale "section.rankings"}}</h2>
      {{if .NoRanking}}<p class="notice">{{.NoRanking}}</p>{{else}}
      <p>{{msg $locale "ranking.warning"}}</p>{{range .Rankings}}{{template "localizedTable" .}}{{end}}{{end}}
    </section>{{end}}
    {{if .FAQ}}<section id="portal-faq"><h2>{{msg $locale "section.faq"}}</h2>
      {{range .FAQ}}<h3>{{.Question}}</h3><p>{{.Answer}}{{if .LinkHref}} <a href="{{.LinkHref}}">{{.LinkLabel}}</a>{{end}}</p>{{end}}
    </section>{{end}}
    {{range .Notes}}<section class="{{if $.Insights}}qa-card{{else}}doc-section{{end}}"{{if .ID}} id="{{.ID}}"{{end}}>
      <h2>{{.Title}}</h2><p>{{.Body}}</p>
      {{if or .Detail .SourceHref $.Insights}}<p class="qa-meta">{{if .Detail}}{{.Detail}}{{else if $.Insights}}{{msg $locale "insights.caveat"}}{{end}}{{if .SourceHref}} <a href="{{.SourceHref}}">{{msg $locale "source.link"}}</a>{{end}}</p>{{end}}
    </section>{{end}}
    {{end}}{{template "modules" .}}
  </main>
  <footer class="footer">
    <div class="wrap">
      <p class="footer-tagline">{{msg $locale "footer"}}</p>
      <div class="footer-cols">{{range .Footer}}
        <nav aria-label="{{.Title}}"><h2>{{.Title}}</h2>{{template "localizedLinks" .Links}}</nav>
      {{end}}</div>
      <div class="footer-legal">
        <p>{{if or .Enforcement .EnforcementRecord}}{{template "localizedFinesSource" $locale}}. {{else}}{{msg $locale "description.source"}} {{end}}{{msg $locale "footer.license"}}</p>
        <p>{{msg $locale "publication.label"}} {{.PublishedAt}} {{if not (or .Enforcement .EnforcementRecord)}} · {{msg $locale "period.label"}} {{.Period}}{{end}}</p>
        <p>{{msg $locale "disclosure"}}</p>
      </div>
    </div>
  </footer>
</body>
</html>
{{end}}

{{define "localizedLinks"}}{{range .}}<a href="{{.Href}}">{{.Label}}{{if .Note}} <small>({{.Note}})</small>{{end}}</a>{{end}}{{end}}

{{define "localizedTable"}}<div class="table-wrap"{{if .ID}} id="{{.ID}}"{{end}}>
  {{if .Detail}}<p class="muted">{{.Detail}}</p>{{end}}
  <table><caption>{{.Title}}</caption>
  <thead><tr>{{range .Headers}}<th scope="col">{{.}}</th>{{end}}</tr></thead>
  <tbody>{{range .Rows}}<tr>{{range .}}<td>{{if .Href}}<a href="{{.Href}}">{{.Text}}</a>{{else}}{{.Text}}{{end}}{{range .Details}}<small class="station-legal-name muted">{{.}}</small>{{end}}</td>{{end}}</tr>{{end}}</tbody>
</table></div>{{end}}

{{define "localizedExplorer"}}{{$locale := .Locale}}{{with .Explorer}}
<section class="explorer" data-portal-prices data-shard-base="/dados/busca/" data-release="{{.Release}}" data-locale="{{$locale.Code}}" data-portal-copy="{{.Copy}}" data-station-prefix="{{.StationPrefix}}" aria-labelledby="explorar-precos">
  <h2 id="explorar-precos">{{msg $locale "explorer.title"}}</h2>
  <div class="form-row">
    <div><label for="prices-fuel">{{msg $locale "col.fuel"}}</label><select id="prices-fuel" data-prices-fuel>{{range .Fuels}}<option value="{{.Code}}" data-fuel-label="{{.Label}}" data-fuel-unit="{{.Unit}}">{{.Label}} ({{.Count}})</option>{{end}}</select></div>
    <div><label for="prices-order">{{msg $locale "explorer.order"}}</label><select id="prices-order" data-prices-order><option value="asc">{{msg $locale "explorer.ascending"}}</option><option value="desc">{{msg $locale "explorer.descending"}}</option></select></div>
    <div class="finder"><label for="prices-q">{{msg $locale "explorer.filter"}}</label><input id="prices-q" data-finder-input type="search" maxlength="80" autocomplete="off" role="combobox" aria-autocomplete="list" aria-expanded="false" aria-controls="prices-suggestions" placeholder="{{msg $locale "explorer.example"}}"><ul id="prices-suggestions" class="suggestions" role="listbox" aria-label="{{msg $locale "explorer.suggestions"}}" data-finder-suggestions hidden></ul></div>
  </div>
  <div class="scope-chip" data-finder-scope hidden></div>
  <p class="muted" data-prices-status role="status" aria-live="polite">{{msg $locale "explorer.noJS"}}</p>
  <div class="table-wrap"><table data-prices-table hidden><caption>{{msg $locale "explorer.caption"}}</caption><thead><tr><th scope="col">{{msg $locale "col.station"}}</th><th scope="col">{{msg $locale "col.place"}}</th><th scope="col" class="num">{{msg $locale "col.price"}}</th><th scope="col">{{msg $locale "col.date"}}</th></tr></thead><tbody data-prices-body></tbody></table></div>
</section>{{end}}{{end}}
`
