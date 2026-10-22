package application

// Suggestion is one visible chip: a short label and the exact supported
// question it resolves. Labels use the reviewed Portuguese copy; technical
// schema names never reach the product surface.
type Suggestion struct {
	Label    string
	Question string
}

// Suggestions lists the frozen supported questions in display order:
// price exploration first, then official records. Freezing the list
// keeps chips, tests and reviewed copy identical.
func Suggestions() []Suggestion {
	return []Suggestion{
		{Label: "Maior preço observado", Question: "Qual o maior preço observado da gasolina comum no Brasil?"},
		{Label: "Maior média por estado", Question: "Qual a maior média de preço do etanol por estado?"},
		{Label: "Menor preço observado", Question: "Qual o menor preço observado do diesel s10 no Brasil?"},
		{Label: "Ações de fiscalização", Question: "Quais ações de fiscalização da ANP no Distrito Federal?"},
		{Label: "Análises de qualidade", Question: "Quais análises de qualidade do combustível em São Paulo?"},
		{Label: "Infrações informadas", Question: "Quais infrações informadas pela fonte no Rio de Janeiro?"},
	}
}
