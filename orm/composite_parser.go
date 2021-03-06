package orm

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/go-pg/pg/internal/parser"
	"github.com/go-pg/pg/types"
)

var endOfComposite = errors.New("pg: end of composite")

type compositeParser struct {
	p parser.StreamingParser

	stickyErr error
}

func newCompositeParserErr(err error) *compositeParser {
	return &compositeParser{
		stickyErr: err,
	}
}

func newCompositeParser(rd types.Reader) *compositeParser {
	p := parser.NewStreamingParser(rd)
	err := p.SkipByte('(')
	if err != nil {
		return newCompositeParserErr(err)
	}
	return &compositeParser{
		p: p,
	}
}

func (p *compositeParser) NextElem() ([]byte, error) {
	if p.stickyErr != nil {
		return nil, p.stickyErr
	}

	c, err := p.p.ReadByte()
	if err != nil {
		if err == io.EOF {
			return nil, endOfComposite
		}
		return nil, err
	}

	switch c {
	case '"':
		return p.readQuoted()
	case ',':
		return nil, nil
	case ')':
		return nil, endOfComposite
	default:
		_ = p.p.UnreadByte()
	}

	var b []byte
	for {
		bb, err := p.p.ReadSlice(',')
		if b == nil {
			b = bb[:len(bb):len(bb)]
		} else {
			b = append(b, bb...)
		}
		if err == nil {
			b = b[:len(b)-1]
			break
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			if b[len(b)-1] == ')' {
				b = b[:len(b)-1]
				break
			}
		}
		return nil, err
	}

	if len(b) == 0 { // NULL
		return nil, nil
	}
	return b, nil
}

func (p *compositeParser) readQuoted() ([]byte, error) {
	var b []byte

	c, err := p.p.ReadByte()
	if err != nil {
		return nil, err
	}

	for {
		next, err := p.p.ReadByte()
		if err != nil {
			return nil, err
		}

		if c == '\\' || c == '\'' {
			if next == c {
				b = append(b, c)
				c, err = p.p.ReadByte()
				if err != nil {
					return nil, err
				}
			} else {
				b = append(b, c)
				c = next
			}
			continue
		}

		if c == '"' {
			switch next {
			case '"':
				b = append(b, '"')
				c, err = p.p.ReadByte()
				if err != nil {
					return nil, err
				}
			case ',', ')':
				return b, nil
			default:
				return nil, fmt.Errorf("pg: got %q, wanted ',' or ')'", c)
			}
			continue
		}

		b = append(b, c)
		c = next
	}
}
