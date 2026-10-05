# Landing Educational Guides Contract (P26)

State: **LOCAL_DONE — APPROVED**, 2026-10-05. Scope: educational evergreen guides content contract, URLs, structure, and factual bounds. Owner: [P26-T01](../../ROADMAP.md#p26-t01) under the [static landing plan](../planning/STATIC_LANDING_PLAN.md).

---

## 1. Scope & Purpose

This contract defines the content, technical architecture, and editorial bounds for three original evergreen educational guides for **abastevo**, along with their dedicated hub index page under `/guias/`.

These guides address authentic public search queries and consumer education regarding fuel pricing in Brazil, establishing domain authority without speculative blog engines or generated doorway pages:
1. **`/guias/`** — Central de Guias Educativos (Hub index)
2. **`/guias/pesquisa-anp/`** — Como funciona a pesquisa semanal de preços da ANP
3. **`/guias/etanol-ou-gasolina/`** — Etanol ou gasolina: a regra dos 70% e o cálculo exato
4. **`/guias/como-ler-precos/`** — Como ler preços de combustíveis: fontes, datas e condições

---

## 2. Editorial & Product Constraints (B-BR-L01 to B-BR-L07)

- **B-BR-L01 (Truthful Presentation):**
  - No claims of "menor preço garantido", "economia mágica de 30%", or "posto mais barato perto de você".
  - Explicit distinction between official regulatory weekly survey data (ANP) and real-time community observations.
  - Transparent statement that abastevo is an independent open-source project and is **not affiliated with the ANP**.
- **B-BR-L04 (Static Essential Content):**
  - All guides, formulas, tables, headings, and breadcrumbs are fully authored in static HTML5.
  - The content remains completely readable and functional with JavaScript disabled or unavailable.
- **B-BR-L05 (Evidence-Based Content & Technical SEO):**
  - Title lengths bounded strictly between 30 and 65 characters.
  - Meta description lengths bounded strictly between 110 and 165 characters.
  - Factual JSON-LD structured data (`Article` / `TechArticle`, `BreadcrumbList`, and `WebSite`).
  - No synthetic author personas or fake user ratings/testimonials.
- **B-BR-L06 (Minimal Data Surface):**
  - Zero cookies, zero third-party telemetry, zero remote font requests.
  - Pure local styling via existing CSS design tokens.
- **Restrained Navigation:**
  - Clear breadcrumb trail on each guide: `Início > Guias > [Título do Guia]`.
  - Quiet footer and discreet contextual link to the main landing page (`/`) and GitHub repository (`https://github.com/AlexandreZanata/abastevo`).
  - Prelaunch store notice preserved ("Em breve na Google Play") without phantom download links.

---

## 3. Guide Specifications

### Guia 1: Central de Guias (`/guias/index.html`)
- **URL Canônica:** `https://abastevo.com.br/guias/`
- **Título SEO:** `Guias Educativos sobre Combustíveis | abastevo` (47 caracteres)
- **Descrição SEO:** `Entenda como funciona a pesquisa da ANP, o cálculo entre etanol e gasolina e como interpretar preços de combustíveis com transparência e dados reais.` (151 caracteres)
- **Conteúdo Principal:**
  - H1: "Guias Educativos abastevo"
  - Subtítulo calmo sobre transparência, consumo consciente e inteligência comunitária na hora de abastecer.
  - 3 cartões de navegação para os guias individuais, com resumo e tempo estimado de leitura (4–6 min).
  - Seção de esclarecimento sobre o projeto: código aberto, dados públicos e colaboração cidadã.

### Guia 2: Pesquisa Semanal da ANP (`/guias/pesquisa-anp/index.html`)
- **URL Canônica:** `https://abastevo.com.br/guias/pesquisa-anp/`
- **Título SEO:** `Como Funciona a Pesquisa de Preços da ANP | abastevo` (51 caracteres)
- **Descrição SEO:** `Entenda a metodologia da pesquisa semanal de combustíveis da ANP, sua amostragem, por que não é tempo real e como a comunidade abastevo complementa os dados.` (159 caracteres)
- **Estrutura de Conteúdo:**
  - H1: "Como funciona a pesquisa semanal de preços de combustíveis da ANP"
  - **O que é a ANP:** Agência Nacional do Petróleo, Gás Natural e Biocombustíveis e seu papel regulador no mercado nacional de combustíveis.
  - **Metodologia de Coleta:** Levantamento semanal de preços por amostragem em municípios selecionados em todo o território nacional.
  - **Periodicidade e Defasagem Temporal:** Por que a pesquisa reflete uma média histórica da semana anterior, e não o preço instantâneo do dia ou da hora.
  - **Diferença entre Referência ANP e Dados Comunitários:** A ANP oferece uma base estatística robusta e regulatória; o abastevo permite que motoristas registrem a realidade atual na bomba.
  - **Acesso aos Dados Abertos e Independência:** Links de referência para os relatórios públicos da ANP; declaração explícita de não afiliação governamental.

### Guia 3: Cálculo Etanol vs Gasolina (`/guias/etanol-ou-gasolina/index.html`)
- **URL Canônica:** `https://abastevo.com.br/guias/etanol-ou-gasolina/`
- **Título SEO:** `Etanol ou Gasolina: Cálculo dos 70% e Eficiência | abastevo` (57 caracteres)
- **Descrição SEO:** `Aprenda o cálculo exato para escolher entre etanol e gasolina, a física da paridade de 70%, quando a regra varia e como medir o consumo do seu veículo flex.` (154 caracteres)
- **Estrutura de Conteúdo:**
  - H1: "Etanol ou gasolina: a regra dos 70% e como calcular com precisão"
  - **A Física da Paridade Energética:** Poder calorífico inferior do etanol hidratado (~70% do calor de combustão da gasolina tipo C com 27% de etanol anidro).
  - **A Fórmula Matemática Simples:** `Preço do Etanol / Preço da Gasolina`. Se o resultado for menor ou igual a `0,70`, o etanol tende a ser economicamente mais viável.
  - **Tabela Ilustrativa de Paridade:** Matriz comparativa com faixas de preço reais (ex: gasolina a R$ 5,80 -> etanol viável até R$ 4,06).
  - **Quando a Regra dos 70% Varia:**
    - Motores flex modernos de alta taxa de compressão e injeção direta: paridade real pode atingir 72% a 75%.
    - Veículos antigos ou uso severo urbano: paridade pode cair para 67% a 68%.
  - **Como Fazer o Teste de Tanque a Tanque:** Método prático de medição no hodômetro para descobrir a paridade exata do próprio veículo.

### Guia 4: Como Interpretar Preços (`/guias/como-ler-precos/index.html`)
- **URL Canônica:** `https://abastevo.com.br/guias/como-ler-precos/`
- **Título SEO:** `Como Ler Preços de Combustíveis na Bomba e Placa | abastevo` (57 caracteres)
- **Descrição SEO:** `Saiba como interpretar preços na placa e na bomba, diferenças entre pagamento à vista e prazo, selos de data e fonte e a segurança do posto de bandeira.` (155 caracteres)
- **Estrutura de Conteúdo:**
  - H1: "Como interpretar preços de combustíveis: fontes, datas e condições"
  - **Preço da Placa vs Preço da Bomba:** O que a legislação brasileira (Decreto 10.634/2021 e normas Procon) exige de clareza nas placas de preços.
  - **Condições de Pagamento:**
    - Diferenciação legal de preços (Lei 13.455/2017) para dinheiro/PIX vs cartão de crédito.
    - Preços atrelados a aplicativos de fidelidade e postos parceiros vs preço geral.
  - **Selos e Fontes no abastevo:**
    - Selo "Referência ANP": dados oficiais consolidados da pesquisa semanal.
    - Selo "Comunidade": observação feita por colaborador com data/hora exata do registro.
  - **Bandeira do Posto vs Bandeira Branca:** Postos vinculados a grandes distribuidoras e postos independentes (obrigatoriedade de informar distribuidora de origem no bico).
  - **Direitos do Consumidor:** Teste de proveta de 20 litros na bomba para aferição volumétrica (norma Inmetro).

---

## 4. Visual Tokens & Accessibility
- **CSS:** Reutilização estrita dos tokens de design aprovados em `landing/static/assets/styles.css`.
- **Tipografia & Legibilidade:** Contraste WCAG AA verificado (mínimo 4.5:1 para texto normal, 3:1 para elementos de interface).
- **Semântica:** `<main>`, `<article>`, `<header>`, `<footer>`, `<nav aria-label="Breadcrumb">`, tabelas com `<caption>`, `<th> scope="col"`.
- **Design Responsivo:** Layout fluido mobile-first com quebras em 48rem e 64rem.
