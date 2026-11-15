package fiscal

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Explicit bounds of the XML envelope check (EDR-159). MaxXMLBytes is the
// BR-PRIV-020 XML upload limit; MaxItems is the NF-e det occurrence limit.
const (
	MaxXMLBytes    = 1 << 20
	MaxXMLDepth    = 32
	MaxXMLElements = 100000
	MaxItems       = 990
)

// XMLSummary is what a STRUCTURE_OK envelope exposes: the key, the item count
// and the exact total. Signatures and protocol status are never read.
type XMLSummary struct {
	redacted
	key   AccessKey
	items int
	total Amount
}

func (x XMLSummary) AccessKey() AccessKey { return x.key }
func (x XMLSummary) Items() int           { return x.items }
func (x XMLSummary) Total() Amount        { return x.total }

const (
	nfeNamespace = "http://www.portalfiscal.inf.br/nfe"
	maxWatched   = 6 // deepest watched path: nfeProc/NFe/infNFe/total/ICMSTot/vNF
)

var byteOrderMark = []byte{0xef, 0xbb, 0xbf}

// Watched paths, relative to the NFe element (or the nfeProc root for "/protNFe...").
var (
	ideLeaves  = []string{"cUF", "cNF", "mod", "serie", "nNF", "dhEmi", "tpEmis", "cDV"}
	singletons = []string{"/infNFe/ide", "/infNFe/emit", "/infNFe/total", "/infNFe/total/ICMSTot", "/infNFe/total/ICMSTot/vNF"}
	atMostOnce = []string{"/infNFe/emit/CNPJ", "/infNFe/emit/CPF"}
	containers = map[string]bool{"": true, "/infNFe": true, "/infNFe/ide": true, "/infNFe/emit": true, "/infNFe/det": true,
		"/infNFe/total": true, "/infNFe/total/ICMSTot": true, "/protNFe": true}
	leafPaths = watchedLeaves()
)

// watchedLeaves are the text-only elements whose first text is kept.
func watchedLeaves() map[string]bool {
	leaves := map[string]bool{"/infNFe/emit/CNPJ": true, "/infNFe/emit/CPF": true, "/infNFe/total/ICMSTot/vNF": true,
		"/protNFe/infProt/chNFe": true}
	for _, leaf := range ideLeaves {
		leaves["/infNFe/ide/"+leaf] = true
	}
	return leaves
}

// scan is what one bounded streaming pass keeps: the supported root's base
// path, the counts and first texts of watched paths and the infNFe attributes.
type scan struct {
	base               string
	counts             map[string]int
	texts              map[string]string
	leafWithChild      bool
	id, version        string
	idCount, verCount  int
	capturing          string
	text               strings.Builder
	stack              []string
	elements, tokenIdx int
}

// CheckXML checks the bounded NF-e/NFC-e XML envelope (B-BR-EMP-234).
// Oversized input is refused before parsing and any declaration is refused
// at its first token without expansion.
func CheckXML(data []byte) Result {
	c := checker{kind: XMLInput}
	c.run(CheckXMLSize)
	if len(data) > MaxXMLBytes {
		return c.fail(ReasonXMLTooLarge)
	}
	c.run(CheckXMLParse)
	sc, reason := parse(bytes.TrimPrefix(data, byteOrderMark))
	if reason != "" {
		return c.fail(reason)
	}
	c.run(CheckXMLRoot)
	if sc.base == "" {
		return c.fail(ReasonXMLRootUnsupported)
	}
	c.run(CheckXMLEnvelope)
	if sc.counts[""] != 1 || sc.counts["/infNFe"] != 1 || sc.idCount != 1 || sc.verCount != 1 || !strings.HasPrefix(sc.id, "NFe") {
		return c.fail(ReasonXMLEnvelope)
	}
	c.run(CheckXMLVersion)
	if sc.version != "4.00" {
		return c.fail(ReasonXMLVersionUnsupported)
	}
	key, reason := c.accessKey(sc.id[3:])
	if reason != "" {
		return c.fail(reason)
	}
	c.run(CheckXMLFields)
	if !sc.fields() {
		return c.fail(ReasonXMLFields)
	}
	c.run(CheckXMLIssuer)
	if reason := sc.issuer(); reason != "" {
		return c.fail(reason)
	}
	c.run(CheckXMLKeyConsistency)
	if !sc.consistent(key) {
		return c.fail(ReasonXMLKeyMismatch)
	}
	c.run(CheckXMLItems)
	items := sc.counts["/infNFe/det"]
	if items < 1 || items > MaxItems {
		return c.fail(ReasonXMLItems)
	}
	c.run(CheckXMLTotal)
	total, ok := parseAmount(sc.texts["/infNFe/total/ICMSTot/vNF"])
	if !ok {
		return c.fail(ReasonXMLTotal)
	}
	out := c.pass()
	out.key, out.xml = &key, &XMLSummary{key: key, items: items, total: total}
	return out
}

// parse is one strict pass of the standard decoder with no entity map and no
// charset reader; it stops at the first offending token.
func parse(data []byte) (*scan, Reason) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	sc := &scan{counts: map[string]int{}, texts: map[string]string{}}
	rootSeen := false
	for ; ; sc.tokenIdx++ {
		token, err := d.Token()
		if err == io.EOF {
			if !rootSeen {
				return nil, ReasonXMLMalformed
			}
			return sc, ""
		}
		if err != nil {
			return nil, ReasonXMLMalformed
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, ReasonXMLDoctype
		case xml.ProcInst:
			if t.Target != "xml" || sc.tokenIdx != 0 {
				return nil, ReasonXMLInstruction
			}
		case xml.StartElement:
			if len(sc.stack) == 0 {
				if rootSeen {
					return nil, ReasonXMLMalformed
				}
				rootSeen = true
				sc.setRoot(t.Name)
			}
			if len(sc.stack) >= MaxXMLDepth {
				return nil, ReasonXMLDepth
			}
			if sc.elements++; sc.elements > MaxXMLElements {
				return nil, ReasonXMLElements
			}
			sc.start(t)
		case xml.EndElement:
			sc.end()
		case xml.CharData:
			if len(sc.stack) == 0 && len(bytes.Trim(t, " \t\r\n")) > 0 {
				return nil, ReasonXMLMalformed
			}
			if sc.capturing != "" {
				sc.text.Write(t)
			}
		}
	}
}

func (sc *scan) setRoot(name xml.Name) {
	if name.Space != nfeNamespace {
		return
	}
	switch name.Local {
	case "nfeProc":
		sc.base = "nfeProc/NFe"
	case "NFe":
		sc.base = "NFe"
	}
}

func (sc *scan) start(t xml.StartElement) {
	name := t.Name.Local
	if t.Name.Space != nfeNamespace {
		name = "\x00" + name
	}
	sc.stack = append(sc.stack, name)
	if sc.capturing != "" {
		sc.leafWithChild = true
	}
	if sc.base == "" || len(sc.stack) > maxWatched {
		return
	}
	path, ok := sc.relative()
	if !ok {
		return
	}
	if !leafPaths[path] && !containers[path] {
		return
	}
	sc.counts[path]++
	if path == "/infNFe" {
		for _, a := range t.Attr {
			switch {
			case a.Name.Space == "" && a.Name.Local == "Id":
				sc.id, sc.idCount = a.Value, sc.idCount+1
			case a.Name.Space == "" && a.Name.Local == "versao":
				sc.version, sc.verCount = a.Value, sc.verCount+1
			}
		}
	}
	if leafPaths[path] && sc.capturing == "" {
		sc.capturing = path
		sc.text.Reset()
	}
}

// relative maps the current stack to a watched path: "" is the NFe element,
// "/..." below it; protocol paths are taken relative to the nfeProc root.
func (sc *scan) relative() (string, bool) {
	full := strings.Join(sc.stack, "/")
	switch {
	case full == sc.base:
		return "", true
	case strings.HasPrefix(full, sc.base+"/"):
		return full[len(sc.base):], true
	case sc.base == "nfeProc/NFe" && (full == "nfeProc/protNFe" || strings.HasPrefix(full, "nfeProc/protNFe/")):
		return full[len("nfeProc"):], true
	}
	return "", false
}

func (sc *scan) end() {
	if sc.capturing != "" && len(sc.stack) <= maxWatched {
		if path, ok := sc.relative(); ok && path == sc.capturing {
			if _, seen := sc.texts[path]; !seen {
				sc.texts[path] = sc.text.String()
			}
			sc.capturing = ""
		}
	}
	sc.stack = sc.stack[:len(sc.stack)-1]
}

// fields checks the version 4.00 cardinalities of the watched elements.
func (sc *scan) fields() bool {
	for _, leaf := range ideLeaves {
		if sc.counts["/infNFe/ide/"+leaf] != 1 {
			return false
		}
	}
	for _, path := range singletons {
		if sc.counts[path] != 1 {
			return false
		}
	}
	for _, path := range atMostOnce {
		if sc.counts[path] > 1 {
			return false
		}
	}
	protocols := sc.counts["/protNFe"]
	if protocols > 1 || (protocols == 1 && sc.counts["/protNFe/infProt/chNFe"] != 1) {
		return false
	}
	return !sc.leafWithChild
}

// issuer accepts exactly 14 uppercase alphanumeric characters that the kernel
// CNPJ type accepts unchanged; a CPF-only issuer is not handled.
func (sc *scan) issuer() Reason {
	switch {
	case sc.counts["/infNFe/emit/CNPJ"] == 1:
		raw := sc.texts["/infNFe/emit/CNPJ"]
		if c, err := kernel.ParseCNPJ(raw); err != nil || c.Normalized() != raw {
			return ReasonXMLIssuer
		}
		return ""
	case sc.counts["/infNFe/emit/CPF"] == 1:
		return ReasonXMLIssuerUnsupported
	}
	return ReasonXMLIssuer
}

// canonical zero-pads a number written without leading zeros (or the single
// "0" when zero is allowed) to width.
func canonical(raw string, width int, zero bool) (string, bool) {
	if !digits(raw) || len(raw) > width || (raw[0] == '0' && (len(raw) > 1 || !zero)) {
		return "", false
	}
	return strings.Repeat("0", width-len(raw)) + raw, true
}

// consistent compares the key with the fields it is composed of.
func (sc *scan) consistent(k AccessKey) bool {
	t := sc.texts
	dh := t["/infNFe/ide/dhEmi"]
	if len(dh) < 8 || !digits(dh[:4]) || dh[4] != '-' || !digits(dh[5:7]) || dh[7] != '-' {
		return false
	}
	serie, okSerie := canonical(t["/infNFe/ide/serie"], 3, true)
	number, okNumber := canonical(t["/infNFe/ide/nNF"], 9, false)
	if chNFe, ok := t["/protNFe/infProt/chNFe"]; ok && chNFe != k.text {
		return false
	}
	return okSerie && okNumber && t["/infNFe/ide/cUF"] == k.State() && dh[2:4]+dh[5:7] == k.YearMonth() &&
		t["/infNFe/emit/CNPJ"] == k.Issuer() && t["/infNFe/ide/mod"] == k.Model() && serie == k.Series() &&
		number == k.Number() && t["/infNFe/ide/tpEmis"] == k.EmissionType() && t["/infNFe/ide/cNF"] == k.NumericCode() &&
		t["/infNFe/ide/cDV"] == k.CheckDigit()
}
