package portalgen

const templates = `
{{define "head"}}<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="robots" content="{{if .Indexable}}index,follow{{else}}noindex,nofollow{{end}}">
  <title>{{.Title}} | abastevo</title>
  <meta name="description" content="{{.Description}}">
  <link rel="stylesheet" href="/assets/portal.css">
</head>
<body>
  <a class="skip" href="#conteudo">Pular para o conteúdo</a>
  <header class="site-header">
    <div class="wrap header-inner">
      <a class="brand" href="/consultar/">abastevo</a>
      <nav class="portal-nav" aria-label="Navegação do portal">
        <a href="/consultar/" {{if eq .ActiveNav "consultar"}}aria-current="page"{{end}}>Consultar</a>
        <a href="/precos/" {{if eq .ActiveNav "precos"}}aria-current="page"{{end}}>Preços</a>
        <a href="/metodologia/" {{if eq .ActiveNav "metodologia"}}aria-current="page"{{end}}>Metodologia</a>
        <a href="/fiscalizacao/" {{if eq .ActiveNav "fiscalizacao"}}aria-current="page"{{end}}>Fiscalização</a>
      </nav>
    </div>
  </header>
{{end}}

{{define "foot"}}
  <footer class="footer">
    <div class="wrap">
      <p><strong>Fonte:</strong> ANP — Levantamento de Preços de Combustíveis. Cálculo e exibição: Abastevo.</p>
      <p>Publicação Abastevo: {{if .PublishedAt}}{{.PublishedAt}}{{else}}não publicada{{end}} · Pesquisa: {{if .Period}}{{.Period}}{{else}}sem pesquisa publicada{{end}}</p>
      <p>Não afiliado à ANP. Código aberto sob licença MIT. Dados datados; não é preço ao vivo.</p>
    </div>
  </footer>
</body>
</html>{{end}}

{{define "crumbs"}}{{if .Breadcrumbs}}<nav class="breadcrumbs" aria-label="Trilha de navegação">{{range $i, $c := .Breadcrumbs}}{{if $i}} · {{end}}{{if $c.Current}}<span aria-current="page">{{$c.Label}}</span>{{else}}<a href="{{$c.Href}}">{{$c.Label}}</a>{{end}}{{end}}</nav>{{end}}{{end}}

{{define "summary"}}{{if .}}<div class="table-wrap">
  <table>
    <caption>Resumo por combustível e unidade; amostra e observações de origem</caption>
    <thead>
      <tr><th scope="col">Combustível</th><th scope="col">Unidade</th><th scope="col" class="num">Amostra</th><th scope="col" class="num">Observações</th><th scope="col" class="num">Mínimo</th><th scope="col" class="num">Máximo</th><th scope="col" class="num">Dispersão</th><th scope="col">Amostra</th></tr>
    </thead>
    <tbody>
      {{range .}}<tr>
        <td>{{.Fuel}}</td><td>{{.Unit}}</td>
        <td class="num">{{.Stations}} {{plural .Stations "posto" "postos"}}</td><td class="num">{{.Observations}}</td>
        <td class="num">{{brl .Min}}</td><td class="num">{{brl .Max}}</td><td class="num">{{brl .Spread}}</td>
        <td>{{if .Insufficient}}<span class="tag tag-warn">amostra pequena</span>{{else}}—{{end}}</td>
      </tr>{{end}}
    </tbody>
  </table>
</div>{{else}}<div class="notice"><strong>Sem resumo nesta pesquisa.</strong><p>Não há observações compatíveis para este recorte.</p></div>{{end}}{{end}}

{{define "ranking"}}<h3>{{.Title}}</h3>
<p class="muted">{{.Population}} {{plural .Population "posto" "postos"}} na amostra.{{if .Truncated}} Exibindo os primeiros; empates no corte: {{.TiesAtCutoff}}.{{end}}</p>
{{if .Rows}}<div class="table-wrap">
  <table>
    <caption>Menores preços encontrados nesta amostra e período; não é preço ao vivo</caption>
    <thead><tr><th scope="col">#</th><th scope="col">Posto</th><th scope="col">Local</th><th scope="col">Status</th><th scope="col" class="num">Preço</th><th scope="col">Coleta</th><th scope="col">Empates</th></tr></thead>
    <tbody>
      {{range .Rows}}<tr>
        <td>{{.Position}}</td>
        <td><a href="/postos/{{.StationID}}/">{{.Name}}</a><br><span class="muted">{{.CNPJ}}</span></td>
        <td>{{.Place}}</td><td>{{.Status}}</td>
        <td class="num">{{brl .Amount}}</td><td>{{.Collected}}</td>
        <td>{{if gt .TieCount 1}}<span class="tag">{{.TieCount}} no mesmo preço</span>{{else}}—{{end}}</td>
      </tr>{{end}}
    </tbody>
  </table>
</div>{{else}}<div class="notice"><strong>Sem postos nesta amostra.</strong></div>{{end}}{{end}}

{{define "consultar"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">Consulta pública</p>
  <h1>Consultar preços de combustíveis</h1>
  <p class="lead">Preços encontrados na pesquisa da ANP, com período, fonte e amostra. Não é preço ao vivo.</p>
  <div class="notice"><strong>Busca por nome/CNPJ</strong><p>Os índices estáticos de busca serão publicados na próxima etapa. Por enquanto, navegue pelos estados abaixo.</p></div>
  <h2>Estados na amostra</h2>
  {{if .States}}<div class="grid cards">{{range .States}}<article class="card"><h3><a href="/precos/uf/{{.Code}}/">{{.Name}} ({{.Code}})</a></h3><p>{{.Stations}} {{plural .Stations "posto" "postos"}} na amostra · {{.Fuels}} combustíveis/unidades</p></article>{{end}}</div>{{else}}<div class="notice"><strong>Sem preços nesta publicação.</strong><p>A pesquisa mais recente não tem observações publicadas.</p></div>{{end}}
  <h2>Resumo nacional</h2>
  {{template "summary" .National}}
</main>
{{template "foot" .}}{{end}}

{{define "precos"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">Resumo por região</p>
  <h1>Preços de combustíveis na pesquisa da ANP</h1>
  <p class="lead">Resumo derivado dos preços de posto da pesquisa mais recente. Não é uma estatística oficial da ANP nem um preço ao vivo.</p>
  <div class="notice"><strong>Leia a amostra antes do número.</strong><p>Mínimo, máximo e dispersão vêm dos postos com observação no período. A amostra não cobre todos os postos e um valor histórico não é uma oferta atual.</p></div>
  <h2>Brasil</h2>
  {{template "summary" .National}}
  <h2>Menores preços encontrados na amostra</h2>
  {{if .Rankings}}{{range .Rankings}}{{template "ranking" .}}{{end}}{{else}}<div class="notice"><strong>Sem ranking nacional.</strong><p>Não há postos suficientes para este recorte.</p></div>{{end}}
  <h2>Estados</h2>
  {{if .States}}<div class="grid cards">{{range .States}}<article class="card"><h3><a href="/precos/uf/{{.Code}}/">{{.Name}} ({{.Code}})</a></h3><p>{{.Stations}} {{plural .Stations "posto" "postos"}} na amostra · {{.Fuels}} combustíveis/unidades</p></article>{{end}}</div>{{else}}<div class="notice"><strong>Sem resumo estadual.</strong></div>{{end}}
</main>
{{template "foot" .}}{{end}}

{{define "state"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">Resumo estadual</p>
  <h1>Preços em {{.Name}} ({{.UF}})</h1>
  <p class="lead">Preços encontrados na pesquisa da ANP para os postos com identidade e observação neste estado.</p>
  <h2>Resumo por combustível</h2>
  {{template "summary" .Summaries}}
  {{if .Rankings}}<h2>Menores preços encontrados na amostra</h2>{{range .Rankings}}{{template "ranking" .}}{{end}}{{else}}<div class="notice"><strong>Sem ranking estadual.</strong></div>{{end}}
  <h2>Municípios</h2>
  {{if .Municipalities}}<div class="grid cards">{{range .Municipalities}}<article class="card"><h3><a href="{{.Href}}">{{.Name}}</a></h3><p>{{.Stations}} {{plural .Stations "posto" "postos"}} na amostra</p>{{if .Insufficient}}<p><span class="tag tag-warn">Amostra insuficiente (mínimo 5)</span></p>{{end}}</article>{{end}}</div>{{else}}<div class="notice"><strong>Sem municípios com resumo.</strong></div>{{end}}
</main>
{{template "foot" .}}{{end}}

{{define "municipality"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">Resumo municipal</p>
  <h1>Preços em {{.Name}}</h1>
  <p class="lead">{{.UF}} · IBGE {{.Code}} · {{.Population}} {{plural .Population "posto" "postos"}} na amostra</p>
  {{if .Insufficient}}
  <div class="notice"><strong>Amostra insuficiente para um resumo municipal.</strong><p>Há {{.Population}} {{plural .Population "posto" "postos"}} com observação; o mínimo definido é 5. Não exibimos ranking ou média municipal abaixo desse mínimo.</p></div>
  <h2>Postos com observação (sem ranking)</h2>
  <ul class="station-list">{{range .Rows}}<li><a href="/postos/{{.StationID}}/">{{.Name}}</a> — {{brl .Amount}} · {{.Collected}} · {{.Status}}</li>{{end}}</ul>
  {{else}}
  <h2>Resumo por combustível</h2>
  {{template "summary" .Summaries}}
  <h2>Menores preços encontrados na amostra</h2>
  {{range .Rankings}}{{template "ranking" .}}{{end}}
  {{end}}
</main>
{{template "foot" .}}{{end}}

{{define "station"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">Detalhe do posto</p>
  <h1>{{.Station.Name}}</h1>
  <p class="lead">{{.Station.Place}} · {{.Station.CNPJ}} · <span class="status">{{.Station.Status}}</span></p>
  <div class="grid summary-grid">
    <section class="card">
      <h2>Identidade e endereço</h2>
      <p>{{if .Station.Address}}{{.Station.Address}}{{else}}Endereço não informado{{end}}</p>
      <p class="muted">{{.Station.Location}}. Coordenadas não são exibidas nesta versão.</p>
      <p class="muted">Identificador canônico: {{.Station.ID}}</p>
      {{if .Station.MunicipalityHref}}<p><a href="{{.Station.MunicipalityHref}}">{{.Station.Municipality}}</a></p>{{else if .Station.StateHref}}<p><a href="{{.Station.StateHref}}">Resumo do estado</a></p>{{end}}
      <p class="muted">O status é informado pela fonte; não é um score de qualidade, confiança ou segurança.</p>
    </section>
    <section class="card">
      <h2>Pesquisa de origem</h2>
      <p>ANP — Levantamento de Preços de Combustíveis</p>
      <p>Período: {{if .Period}}{{.Period}}{{else}}sem pesquisa publicada{{end}}</p>
      <p>Publicação Abastevo: {{if .PublishedAt}}{{.PublishedAt}}{{else}}não publicada{{end}}</p>
      <p class="muted">Preços encontrados na amostra; não é preço ao vivo.</p>
    </section>
  </div>
  <h2>Preços encontrados nesta pesquisa</h2>
  {{if .Observations}}<div class="table-wrap"><table>
    <caption>Observações mais recentes por combustível e unidade, com data de coleta</caption>
    <thead><tr><th scope="col">Combustível</th><th scope="col">Unidade</th><th scope="col">Condição</th><th scope="col" class="num">Preço</th><th scope="col">Coleta</th><th scope="col">Fonte</th></tr></thead>
    <tbody>{{range .Observations}}<tr><td>{{.Fuel}}</td><td>{{.Unit}}</td><td>{{.Condition}}</td><td class="num">{{brl .Amount}}</td><td>{{.Collected}}</td><td>ANP</td></tr>{{end}}</tbody>
  </table></div>{{else}}<div class="notice"><strong>Sem preço ANP nesta pesquisa.</strong><p>O posto pode não ter sido amostrado ou pode não ter uma observação válida no período. A ausência não significa preço zero.</p></div>{{end}}
  <h2>Histórico por pesquisa</h2>
  {{if .Histories}}{{range .Histories}}<h3>{{.Fuel}} · {{.Unit}}</h3><div class="table-wrap"><table>
    <caption>Uma linha por pesquisa; lacunas não são preço zero</caption>
    <thead><tr><th scope="col">Período da pesquisa</th><th scope="col" class="num">Preço</th><th scope="col">Coleta</th><th scope="col">Situação</th></tr></thead>
    <tbody>{{range .Points}}<tr><td>{{.Period}}</td><td class="num">{{brlPtr .Amount}}</td><td>{{if .Collected}}{{.Collected}}{{else}}—{{end}}</td><td>{{if .Missing}}Sem observação nesta pesquisa{{else}}Observação publicada{{end}}</td></tr>{{end}}</tbody>
  </table></div>{{end}}{{else}}<div class="notice"><strong>Sem histórico publicado.</strong><p>Não há observações anteriores para este posto.</p></div>{{end}}
  <div class="source">
    <p><strong>Fonte:</strong> ANP — Levantamento de Preços de Combustíveis. {{if .Source.URL}}<a href="{{.Source.URL}}">Página oficial da pesquisa</a>.{{end}}</p>
    <p class="muted source-url">Revisão: {{if .Source.Period}}{{.Source.Period}}{{else}}não informada{{end}} · checksum: {{if .Source.Checksum}}{{.Source.Checksum}}{{else}}não informado{{end}} · parser: {{if .Source.ParserVersion}}{{.Source.ParserVersion}}{{else}}não informado{{end}} · publicação na fonte: {{if .Source.PublishedAt}}{{.Source.PublishedAt}}{{else}}não informada{{end}}</p>
  </div>
  <div class="links"><a href="/consultar/">Voltar à consulta</a><a href="/precos/">Resumo por região</a><a href="/metodologia/">Metodologia e fontes</a></div>
</main>
{{template "foot" .}}{{end}}

{{define "metodologia"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">Transparência</p>
  <h1>Metodologia e fontes</h1>
  <p class="lead">Como os preços, resumos e lacunas são construídos — e o que este portal não afirma.</p>
  <h2>Fontes</h2>
  <ul>
    <li><strong>Cadastro de postos:</strong> dados cadastrais de revendedores da ANP, reutilizados pelo Directory. UUID canônico e CNPJ completo são preservados; um CNPJ inválido não é inventado.</li>
    <li><strong>Preços:</strong> Levantamento de Preços de Combustíveis da ANP, arquivo de posto por semana. Período e data de coleta vêm da fonte.</li>
    <li><strong>Fiscalização e multas:</strong> fatos oficiais separados, ainda não admitidos. Não há score de risco.</li>
  </ul>
  <h2>Datas e relógios</h2>
  <ul>
    <li><strong>Coleta:</strong> data em que a ANP coletou o preço.</li>
    <li><strong>Pesquisa:</strong> semana de referência (início e fim).</li>
    <li><strong>Publicação Abastevo:</strong> quando o arquivo estático foi gerado.</li>
    <li><strong>Última verificação da fonte:</strong> separada da publicação; uma geração nova não rejuvenece a fonte.</li>
  </ul>
  <p>Um preço é sempre “encontrado na pesquisa de [período]”. Não é preço ao vivo e não garante disponibilidade atual.</p>
  <h2>Amostra e rankings</h2>
  <ul>
    <li>A pesquisa é amostral; não cobre todos os postos nem todos os dias.</li>
    <li>Rankings são “menores preços encontrados nesta amostra e período”, com contagem de postos, empates e exclusões.</li>
    <li>Um município só recebe ranking com pelo menos 5 postos com observação; abaixo disso, mostramos amostra insuficiente.</li>
    <li>Semana ausente é lacuna, nunca preço zero. Um mínimo histórico não é uma oferta atual.</li>
  </ul>
  <h2>Dados oficiais e cálculos Abastevo</h2>
  <p>“Fonte: ANP” identifica o fato oficial. “Cálculo Abastevo” identifica mínimo, máximo, dispersão, médias e rankings derivados. Um resumo derivado não é uma estatística oficial da ANP e não é calculado pela média das médias municipais.</p>
  <h2>Correções e retiradas</h2>
  <p>Uma correção da fonte gera uma nova revisão; páginas, resumos e índices são reconstruídos a partir dela. Registros retirados não voltam por rollback. O portal não promete apagar cópias já baixadas por terceiros.</p>
  <h2>Privacidade e independência</h2>
  <p>Nenhuma consulta chama a API do Abastevo, a API da ANP ou o banco. O navegador baixa apenas arquivos estáticos públicos. Não publicamos CPF, fotos de usuários, GPS de colaboradores, documentos internos ou dados de conta. Pagamento ou patrocínio não altera a ordem dos preços nem os fatos oficiais.</p>
  <h2>Bloqueios atuais</h2>
  <ul>
    <li>A publicação pública de preços reais depende de direitos compatíveis e de uma política registrada de precisão monetária para preços com mais de três casas decimais.</li>
    <li>A fiscalização depende da aceitação do fluxo oficial e da associação segura ao posto; até lá, a página explica a ausência e não inventa registros.</li>
  </ul>
  <div class="source">
    <p><strong>Atribuição:</strong> Fonte ANP — Levantamento de Preços de Combustíveis. Cálculo e exibição: Abastevo. Não afiliado à ANP. Código sob licença MIT.</p>
    <p class="muted source-url">Snapshot: {{.InputHash}} · versão dos cálculos: abastevo-metrics-v1 · publicação: {{if .PublishedAt}}{{.PublishedAt}}{{else}}não publicada{{end}}</p>
  </div>
</main>
{{template "foot" .}}{{end}}

{{define "fiscalizacao"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">Registros oficiais</p>
  <h1>Fiscalização — ainda não disponível</h1>
  <p class="lead">Este recorte depende da aceitação do fluxo oficial de fiscalização e da associação segura ao posto. Até lá, nenhum registro é exibido.</p>
  <div class="notice"><strong>Por que a página está vazia</strong><p>O adaptador oficial existe como contrato local, mas o fluxo ao vivo, a cobertura completa e a associação segura ao posto ainda não foram aceitos. Publicar uma lista parcial ou inventar registros seria pior do que dizer “ainda não disponível”.</p></div>
  <h2>O que será exibido quando o recorte for aceito</h2>
  <ul>
    <li>Tipo e status exatamente como informados pela fonte (ação, auto, processo, multa ou decisão), com data e precisão de extração.</li>
    <li>Referência ao documento e ao período, além do escopo da empresa atual versus histórico comprovado do estabelecimento.</li>
    <li>Aviso de que uma multa ou ação não prova adulteração, interdição atual ou culpa definitiva; ausência de registro não é certidão negativa.</li>
    <li>Nenhum CPF, documento interno, foto de usuário ou score de risco.</li>
  </ul>
  <p>A indisponibilidade deste recorte não desabilita a consulta de preços, o resumo regional ou a metodologia.</p>
  <div class="links"><a href="/consultar/">Consultar preços</a><a href="/metodologia/">Metodologia e fontes</a></div>
</main>
{{template "foot" .}}{{end}}

{{define "notfound"}}{{template "head" .}}{{template "crumbs" .}}
<main id="conteudo" class="wrap">
  <p class="eyebrow">404</p>
  <h1>Página não encontrada</h1>
  <p class="lead">A página ou o posto solicitado não existe neste portal estático.</p>
  <div class="notice"><strong>O que fazer</strong><p>Volte à consulta para navegar pelos estados e postos com observação. Um UUID desconhecido não é redirecionado para outro posto.</p></div>
  <div class="links"><a href="/consultar/">Consultar preços</a><a href="/metodologia/">Metodologia e fontes</a></div>
</main>
{{template "foot" .}}{{end}}
`
