package portalgen

const stationForumTemplates = `
{{define "localizedStation"}}{{$locale := .Locale}}{{$page := .}}{{with .Station}}
  <h1>{{$page.Title}}</h1>
  {{if .View.Registry}}<p class="station-legal-name muted">{{msg $locale "station.anpName"}}: {{.View.Registry}}</p>{{end}}
  {{if .View.LegalName}}<p class="station-legal-name muted">{{.View.LegalName}}</p>{{end}}
  <p class="lead">{{.View.Place}} · {{.View.CNPJ}} · <span class="status">{{.View.Status}}</span></p>
  <div class="grid summary-grid">
    <section class="card">
      <h2>{{msg $locale "station.identity"}}</h2>
      <p>{{if .View.Address}}{{.View.Address}}{{else}}{{msg $locale "station.noAddress"}}{{end}}</p>
      <p class="muted">{{.View.Location}}. {{msg $locale "station.noCoordinates"}}</p>
      <p class="muted">{{msg $locale "station.identifier" "id" .View.ID}}</p>
      {{if .Region.Href}}<p><a href="{{.Region.Href}}">{{.Region.Label}}</a>{{if .Region.Note}} <small>({{.Region.Note}})</small>{{end}}</p>{{end}}
      <p class="muted">{{msg $locale "station.sourceStatus"}}</p>
    </section>
    <section class="card">
      <h2>{{msg $locale "station.survey"}}</h2>
      <p>{{msg $locale "agency"}}</p>
      <p>{{msg $locale "period.label"}} {{$page.Period}}</p>
      <p>{{msg $locale "publication.label"}} {{$page.PublishedAt}}</p>
      <p class="muted">{{msg $locale "price.note"}}</p>
    </section>
  </div>
  <h2>{{msg $locale "station.observations"}}</h2>
  {{if .Observations.Rows}}{{template "localizedTable" .Observations}}{{else}}<div class="notice"><strong>{{msg $locale "station.noObservation"}}</strong><p>{{msg $locale "station.noObservationDetail"}}</p></div>{{end}}
  <h2>{{msg $locale "station.history"}}</h2>
  {{range .Histories}}{{template "localizedTable" .}}{{else}}<div class="notice"><strong>{{msg $locale "station.noHistory"}}</strong><p>{{msg $locale "station.noHistoryDetail"}}</p></div>{{end}}
  {{if .Records}}<section>
    <h2>{{msg $locale "station.records"}}</h2>
    <p class="muted">{{msg $locale "station.recordsNote"}}</p>
    <ul class="station-list">{{range .Records}}<li><a href="{{.Link.Href}}" lang="pt-BR">{{.Link.Label}}</a>{{if .Link.Note}} <small>({{.Link.Note}})</small>{{end}}{{if .Proceeding}} — {{.Proceeding}}{{end}}{{if .Amount}} · {{.Amount}}{{end}}{{if .Date}} · {{.Date}}{{end}} · {{.Association}}</li>{{end}}</ul>
    {{if .RecordsSummary}}<p class="muted">{{.RecordsSummary}}</p>{{end}}
  </section>{{end}}
  <div class="source">
    <p>{{msg $locale "description.source"}}{{if .Source.URL}} <a href="{{.Source.URL}}" hreflang="pt-BR">{{msg $locale "station.officialSurvey"}}</a>.{{end}}</p>
    <p class="muted source-url">{{msg $locale "station.revision"}} {{.Source.Period}} · {{msg $locale "station.checksum"}} {{if .Source.Checksum}}{{.Source.Checksum}}{{else}}{{msg $locale "unknown"}}{{end}} · {{msg $locale "station.parser"}} {{if .Source.ParserVersion}}{{.Source.ParserVersion}}{{else}}{{msg $locale "unknown"}}{{end}} · {{msg $locale "station.sourcePublished"}} {{if .Source.PublishedAt}}{{.Source.PublishedAt}}{{else}}{{msg $locale "unknown"}}{{end}}</p>
  </div>
{{end}}{{end}}

{{define "localizedForum"}}{{$locale := .Locale}}{{$page := .}}{{with .Forum}}
  {{if .Post}}{{with .Post}}
    <article class="forum-post">
      <p class="eyebrow"><a href="{{.CategoryHref}}">{{.Category}}</a></p>
      <h1 lang="{{.Language}}">{{.Title}}</h1>
      <p class="post-meta muted">{{msg $locale "forum.by"}} {{if .AboutHref}}<a href="{{.AboutHref}}">{{.Author}}</a>{{else}}{{.Author}}{{end}} · {{msg $locale "forum.published"}} <time datetime="{{.PublishedISO}}">{{.PublishedDay}}</time>{{if .UpdatedISO}} · {{msg $locale "forum.updated"}} <time datetime="{{.UpdatedISO}}">{{.UpdatedDay}}</time>{{end}}</p>
      {{if .Related}}<ul class="chips" aria-label="{{msg $locale "section.related"}}">{{range .Related}}<li><a class="tag" href="{{.Href}}">{{.Label}}{{if .Note}} <small>({{.Note}})</small>{{end}}</a></li>{{end}}</ul>{{end}}
      {{if eq .Language "pt-BR"}}<p class="notice">{{msg $locale "forum.original"}}</p>{{end}}
      <div class="post-body" lang="{{.Language}}"{{if eq .Language "pt-BR"}} data-forum-original{{end}}>{{range .Blocks}}{{if eq .Kind "heading"}}
        <h2>{{.Text}}</h2>{{else if eq .Kind "bullets"}}
        <ul>{{range .Items}}<li>{{.}}</li>{{end}}</ul>{{else}}
        <p>{{range $i, $line := .Lines}}{{if $i}}<br>{{end}}{{$line}}{{end}}</p>{{end}}{{end}}
      </div>
    </article>
    <div class="source"><p>{{msg $locale "forum.source" "author" .Author}}</p></div>
  {{end}}{{else}}
    <p class="eyebrow">{{if eq .Kind "hub"}}{{.Team}}{{else}}{{.Brand}}{{end}}</p>
    <h1>{{$page.Title}}</h1>
    {{if $page.Lead}}<p class="lead">{{$page.Lead}}</p>{{end}}
    <h2>{{.ListTitle}}</h2>
    <ul class="station-list forum-list">{{range .Posts}}<li><a href="{{.Route}}" lang="{{.Language}}">{{.Title}}</a> <span class="muted">{{.Category}} · {{.Date}}{{if eq .Language "pt-BR"}} · {{msg $locale "language.portuguese"}}{{end}}</span>{{if .Excerpt}}<br><span class="muted" lang="{{.Language}}">{{.Excerpt}}</span>{{end}}</li>{{end}}</ul>
    {{if .Limit}}<p class="muted">{{.Limit}}</p>{{end}}
    {{if .Categories}}<h2>{{msg $locale "forum.categories"}}</h2><ul class="station-list">{{range .Categories}}<li><a href="{{.Href}}">{{.Label}}</a> <span class="muted">{{.Note}}</span></li>{{end}}</ul>{{end}}
  {{end}}
  {{if .AppHref}}<p><a href="{{.AppHref}}">{{msg $locale "forum.app"}}</a></p>{{end}}
{{end}}{{end}}
`
