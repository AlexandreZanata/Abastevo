package fiscal

// KeyLength is the fixed NF-e/NFC-e access key length.
const KeyLength = 44

// AccessKey is a structurally checked 44-character access key. Its fields are
// exact substrings; a well-formed key proves no authorization (BR-TRUTH-002).
type AccessKey struct {
	redacted
	text string
}

func (k AccessKey) Text() string { return k.text }
func (k AccessKey) part(from, to int) string {
	if len(k.text) != KeyLength {
		return ""
	}
	return k.text[from:to]
}
func (k AccessKey) State() string        { return k.part(0, 2) }
func (k AccessKey) YearMonth() string    { return k.part(2, 6) }
func (k AccessKey) Issuer() string       { return k.part(6, 20) }
func (k AccessKey) Model() string        { return k.part(20, 22) }
func (k AccessKey) Series() string       { return k.part(22, 25) }
func (k AccessKey) Number() string       { return k.part(25, 34) }
func (k AccessKey) EmissionType() string { return k.part(34, 35) }
func (k AccessKey) NumericCode() string  { return k.part(35, 43) }
func (k AccessKey) CheckDigit() string   { return k.part(43, 44) }

// ufCodes are the 27 IBGE state codes that open an access key.
var ufCodes = map[string]bool{
	"11": true, "12": true, "13": true, "14": true, "15": true, "16": true, "17": true, "21": true, "22": true,
	"23": true, "24": true, "25": true, "26": true, "27": true, "28": true, "29": true, "31": true, "32": true,
	"33": true, "35": true, "41": true, "42": true, "43": true, "50": true, "51": true, "52": true, "53": true,
}

// CheckAccessKey checks the access-key profile, check digit and fields
// (B-BR-EMP-232). Nothing is trimmed, case-folded or reformatted.
func CheckAccessKey(text string) Result {
	c := checker{kind: AccessKeyInput}
	key, reason := c.accessKey(text)
	if reason != "" {
		return c.fail(reason)
	}
	out := c.pass()
	out.key = &key
	return out
}

// keyDigit is the modulo-11 digit of the first 43 characters, each valued as
// its ASCII code minus 48 (NT 2025.001), weights 2..9 repeating from the right.
func keyDigit(body string) byte {
	sum, weight := 0, 2
	for i := len(body) - 1; i >= 0; i-- {
		sum += int(body[i]-'0') * weight
		if weight++; weight > 9 {
			weight = 2
		}
	}
	if r := sum % 11; r > 1 {
		return byte('0' + 11 - r)
	}
	return '0'
}

// accessKey runs KEY_FORMAT, KEY_CHECK_DIGIT and KEY_FIELDS in order.
func (c *checker) accessKey(text string) (AccessKey, Reason) {
	c.run(CheckKeyFormat)
	for i := 0; i < len(text); i++ {
		if b := text[i]; (b < '0' || b > '9') && (b < 'A' || b > 'Z') {
			return AccessKey{}, ReasonKeyCharacter
		}
	}
	if len(text) != KeyLength {
		return AccessKey{}, ReasonKeyLength
	}
	if !digits(text[:6]) || !digits(text[18:]) {
		return AccessKey{}, ReasonKeyCharacter
	}
	c.run(CheckKeyDigit)
	if keyDigit(text[:43]) != text[43] {
		return AccessKey{}, ReasonKeyCheckDigit
	}
	c.run(CheckKeyFields)
	if !ufCodes[text[:2]] {
		return AccessKey{}, ReasonKeyState
	}
	if month := text[4:6]; month < "01" || month > "12" {
		return AccessKey{}, ReasonKeyMonth
	}
	if model := text[20:22]; model != "55" && model != "65" {
		return AccessKey{}, ReasonKeyModelUnsupported
	}
	return AccessKey{text: text}, ""
}
