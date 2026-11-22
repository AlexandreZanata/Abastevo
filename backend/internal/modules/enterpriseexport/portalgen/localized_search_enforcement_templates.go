package portalgen

const searchEnforcementTemplates = `
{{define "localizedSearch"}}{{$locale := .Locale}}{{$route := .Route}}{{with .Search}}
  <form class="finder-form" data-portal-finder data-shard-base="/dados/busca/" data-release="{{.Release}}" data-locale="{{$locale.Code}}" data-portal-copy="{{.Copy}}" data-station-prefix="{{.StationPrefix}}" action="{{$route}}" method="get" role="search">
    <label for="portal-q">{{msg $locale "search.input"}}</label>
    <div class="finder">
      <input id="portal-q" name="q" data-finder-input type="search" maxlength="80" autocomplete="off" role="combobox" aria-autocomplete="list" aria-expanded="false" aria-controls="portal-suggestions" placeholder="{{msg $locale "search.example"}}">
      <button class="button" type="submit">{{msg $locale "nav.search"}}</button>
      <ul id="portal-suggestions" class="suggestions" role="listbox" aria-label="{{msg $locale "explorer.suggestions"}}" data-finder-suggestions hidden></ul>
    </div>
    <div class="scope-chip" data-finder-scope hidden></div>
    <p class="muted" data-finder-status role="status" aria-live="polite">{{msg $locale "search.instructions"}}</p>
    <ul class="station-list" data-finder-results aria-label="{{msg $locale "search.results"}}"></ul>
  </form>
  <noscript><p class="muted">{{msg $locale "search.noJS"}}</p></noscript>
  <h2>{{msg $locale "search.states"}}</h2>
  {{if .States}}
  <div class="grid cards">{{range .States}}<article class="card"><h3><a href="{{.Href}}">{{.Label}}</a></h3><p>{{.Note}}</p></article>{{end}}</div>
  {{else}}<p class="notice">{{msg $locale "price.empty"}}</p>{{end}}
{{end}}{{end}}

{{define "localizedEnforcement"}}{{$locale := .Locale}}{{with .Enforcement}}{{if .HasData}}
  <p class="lead">{{.Lead}}</p>
  <div class="grid summary-grid">
    {{range .Stats}}<section class="card"><h2>{{.Label}}</h2><p class="price">{{.Count}}</p><p class="muted">{{.Detail}}</p></section>{{end}}
  </div>
  <div class="notice"><strong>{{msg $locale "fiscal.read"}}</strong><p>{{msg $locale "fiscal.limits"}}</p></div>
  <div class="source">
    <p>{{template "localizedFinesSource" $locale}}. {{.Edition}}.</p>
    <details><summary>{{msg $locale "section.publication"}}</summary><p class="muted source-url">{{.Publication}}</p></details>
  </div>
  {{if .Release}}<section class="explorer" data-portal-fiscal data-shard-base="/dados/busca/" data-release="{{.Release}}" data-locale="{{$locale.Code}}" data-portal-copy="{{.Copy}}" data-record-prefix="{{.RecordPrefix}}" aria-labelledby="buscar-registros">
    <h2 id="buscar-registros">{{msg $locale "fiscal.search.title"}}</h2>
    <p class="muted">{{msg $locale "fiscal.search.instructions"}}</p>
    <div class="fiscal-filters">
      <div><label for="fiscal-state">{{msg $locale "explorer.state"}}</label><select id="fiscal-state" data-fiscal-state disabled><option value="">{{msg $locale "fiscal.search.loadingStates"}}</option></select></div>
      <div><label for="fiscal-city">{{msg $locale "explorer.city"}}</label><select id="fiscal-city" data-fiscal-city disabled><option value="">{{msg $locale "fiscal.runtime.selectState"}}</option></select></div>
      <button class="button button-secondary" type="button" data-fiscal-clear>{{msg $locale "fiscal.search.clear"}}</button>
    </div>
    <div class="finder"><label for="fiscal-q">{{msg $locale "explorer.filter"}}</label><input id="fiscal-q" data-finder-input type="search" maxlength="80" autocomplete="off" role="combobox" aria-autocomplete="list" aria-expanded="false" aria-controls="fiscal-suggestions" placeholder="{{msg $locale "fiscal.search.example"}}"><ul id="fiscal-suggestions" class="suggestions" role="listbox" aria-label="{{msg $locale "explorer.suggestions"}}" data-finder-suggestions hidden></ul></div>
    <div class="scope-chip" data-finder-scope hidden></div>
    <p class="muted" data-fiscal-status role="status" aria-live="polite">{{msg $locale "fiscal.search.noJS"}}</p>
    <ul class="station-list" data-fiscal-results aria-label="{{msg $locale "fiscal.search.results"}}"></ul>
    <button class="button" type="button" data-fiscal-more hidden>{{msg $locale "fiscal.search.more"}}</button>
  </section>{{end}}
  <section data-fiscal-directory>
    <h2>{{.DirectoryTitle}}</h2>
    {{if .Records.Rows}}{{template "localizedTable" .Records}}{{else}}<p class="notice">{{msg $locale "fiscal.directory.empty"}}</p>{{end}}
    <div class="links">{{if .PrevHref}}<a href="{{.PrevHref}}">{{msg $locale "fiscal.previous"}}</a>{{end}}{{if .NextHref}}<a href="{{.NextHref}}">{{msg $locale "fiscal.next"}}</a>{{end}}</div>
  </section>
{{else}}
  <div class="notice"><strong>{{msg $locale "fiscal.unavailable.title"}}</strong><p>{{msg $locale "fiscal.unavailable.body"}}</p></div>
  <h2>{{msg $locale "fiscal.unavailable.heading"}}</h2>
  <ul>
    <li>{{msg $locale "fiscal.unavailable.fields"}}</li>
    <li>{{msg $locale "fiscal.unavailable.company"}}</li>
    <li>{{msg $locale "fiscal.unavailable.caveat"}}</li>
    <li>{{msg $locale "fiscal.unavailable.privacy"}}</li>
  </ul>
  <p>{{msg $locale "fiscal.unavailable.related"}}</p>
{{end}}{{end}}{{end}}

{{define "localizedEnforcementRecord"}}{{$locale := .Locale}}{{with .EnforcementRecord}}
  <p class="lead">CNPJ {{.CNPJ}} · {{.Association}}</p>
  <div class="grid summary-grid">
    <section class="card"><h2>{{msg $locale "col.association"}}</h2>
      {{if .Station.Href}}<p><a href="{{.Station.Href}}">{{.Station.Label}}</a>{{if .Station.Note}}<small class="station-legal-name muted">{{.Station.Note}}</small>{{end}}</p>{{else}}<p>{{.Association}}.</p>{{end}}
      <p class="muted">{{.Reason}}</p><p class="muted">{{msg $locale "fiscal.record.scope"}}</p>
    </section>
    <section class="card"><h2>{{msg $locale "fiscal.record.sourceDates"}}</h2>
      <p>{{.Edition}}</p><p>{{msg $locale "fiscal.record.extraction"}} {{.Extraction}}</p><p>{{msg $locale "publication.label"}} {{.Publication}}</p><p class="muted">{{template "localizedFinesSource" $locale}}</p>
    </section>
  </div>
  <h2>{{msg $locale "fiscal.record.details"}}</h2>
  <div class="card">{{range .Fields}}<p><strong>{{.Label}}</strong> {{.Value}}</p>{{end}}</div>
  <div class="notice"><strong>{{msg $locale "fiscal.record.limits"}}</strong><p>{{msg $locale "fiscal.limits"}}</p></div>
  <div class="source"><p>{{template "localizedFinesSource" $locale}}{{if .SourceURL}} · <a href="{{.SourceURL}}">{{msg $locale "fiscal.record.sourcePage"}}</a>{{end}}</p><p class="muted source-url">{{.Metadata}}</p></div>
  <div class="links"><a href="{{.BackHref}}">{{msg $locale "fiscal.back"}}</a></div>
{{end}}{{end}}

{{define "localizedFinesSource"}}<strong>{{msg . "source"}}</strong> <a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos">{{msg . "agency"}}</a> — {{msg . "fiscal.fines"}}{{end}}
`
