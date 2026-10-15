package glpi

import "testing"

func TestDecodeHTMLEntities(t *testing.T) {
	cases := map[string]string{
		"Office 365 &#38; Demais Aplicativos &#62; Office 365": "Office 365 & Demais Aplicativos > Office 365",
		"Protheus &amp;#62; Fiscal":                            "Protheus > Fiscal",
		"Texto normal":                                         "Texto normal",
		"&lt;script&gt;":                                       "<script>",
	}
	for in, want := range cases {
		if got := parseString(in); got != want {
			t.Errorf("parseString(%q) = %q, want %q", in, got, want)
		}
	}
}
