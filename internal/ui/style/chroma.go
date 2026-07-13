package style

import (
	"bytes"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/Sh-ui/ghab/internal/config"
)

// chromaColor resolves a palette name to a hex color, falling back to the
// compiled palette if the resolved palette is missing that name --
// fail-soft, matching the rest of the config port. It never returns "".
func chromaColor(palette config.Palette, name string) string {
	if hex, ok := palette[name]; ok && hex != "" {
		return hex
	}
	return config.CompiledFallbackPalette()[name]
}

// ChromaStyle builds the code tab's syntax-highlight style from the
// resolved ombre palette: keyword=purple, string=green, comment=slate,
// number=orange, func=blue, type=teal, error=red -- mirroring
// styles/readme-ombre-dark.json's "chroma" block (BUILD.md [readme]). One
// mapping serves BOTH color modes: ombre accents are luma-balanced for
// either ground. Text/name/punctuation tokens are left uncolored so they
// fall through to the terminal's ANSI foreground -- house rule: no pinned
// neutrals in ui code.
func ChromaStyle(palette config.Palette) *chroma.Style {
	c := func(name string) string { return chromaColor(palette, name) }

	st, err := chroma.NewStyle("ombre", chroma.StyleEntries{
		chroma.Keyword:            c("purple"),
		chroma.KeywordConstant:    c("purple"),
		chroma.KeywordDeclaration: c("purple"),
		chroma.KeywordNamespace:   c("purple"),
		chroma.KeywordReserved:    c("purple"),
		chroma.KeywordType:        c("teal"),
		chroma.NameFunction:       c("blue"),
		chroma.NameClass:          c("teal"),
		chroma.NameBuiltin:        c("teal"),
		chroma.NameTag:            c("blue"),
		chroma.NameAttribute:      c("teal"),
		chroma.NameDecorator:      c("pink"),
		chroma.NameException:      c("red"),
		chroma.LiteralString:      c("green"),
		chroma.LiteralNumber:      c("orange"),
		chroma.LiteralDate:        c("green"),
		chroma.Comment:            c("slate"),
		chroma.CommentPreproc:     c("orange"),
		chroma.GenericDeleted:     c("red"),
		chroma.GenericInserted:    c("green"),
		chroma.Error:              c("red"),
	})
	if err != nil {
		// The entries above are static hex strings we control; NewStyle
		// can only fail on a malformed descriptor, which would be a
		// programming error, not a runtime config problem -- fall back to
		// chroma's bundled default rather than crash the TUI.
		return styles.Fallback
	}
	return st
}

// Highlight tokenizes source (a lexer is picked from filename, falling
// back to content-sniffing, then chroma's plaintext lexer) and renders it
// with th.Chroma through chroma's truecolor ANSI formatter.
func Highlight(th Theme, filename, source string) (string, error) {
	l := lexers.Match(filename)
	if l == nil {
		l = lexers.Analyse(source)
	}
	if l == nil {
		l = lexers.Fallback
	}
	l = chroma.Coalesce(l)

	it, err := l.Tokenise(nil, source)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := formatters.TTY16m.Format(&buf, th.Chroma, it); err != nil {
		return "", err
	}
	return buf.String(), nil
}
