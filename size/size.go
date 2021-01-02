package size

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const Byte = int64(1)

const (
	KiByte = Byte << ((iota + 1) * 10)
	MiByte
	GiByte
	TiByte
	PiByte
	EiByte
)

const (
	KByte = Byte * 1000
	MByte = KByte * 1000
	GByte = MByte * 1000
	TByte = GByte * 1000
	PByte = TByte * 1000
	EByte = PByte * 1000
)

var units = map[string]int64{
	"":    Byte,
	"b":   Byte,
	"k":   KByte,
	"m":   MByte,
	"g":   GByte,
	"t":   TByte,
	"p":   PByte,
	"e":   EByte,
	"kb":  KByte,
	"mb":  MByte,
	"gb":  GByte,
	"tb":  TByte,
	"pb":  PByte,
	"eb":  EByte,
	"ki":  KiByte,
	"mi":  MiByte,
	"gi":  GiByte,
	"ti":  TiByte,
	"pi":  PiByte,
	"ei":  EiByte,
	"kib": KiByte,
	"mib": MiByte,
	"gib": GiByte,
	"tib": TiByte,
	"pib": PiByte,
	"eib": EiByte,
}

var errSyntax = fmt.Errorf("invalid syntax")

func FormatBytes(value int64) string {
	return format(value, KByte, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"})
}

func FormatIBytes(value int64) string {
	return format(value, KiByte, []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"})
}

func format(value, base int64, units []string) string {
	if value < base {
		return fmt.Sprintf("%d %s", value, units[0])
	}
	div, exp := int64(base), 0
	for value := value / base; value >= base; value /= base {
		div *= base
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(value)/float64(div), units[exp+1])
}

func Parse(value string) (int64, error) {
	p := parser{}
	return p.parse(value)
}

type parser struct {
	r *bytes.Buffer
}

func (p *parser) parse(input string) (int64, error) {
	if input == "" {
		return 0, nil
	}

	p.r = bytes.NewBufferString(input)
	var (
		number, unit int64
		err          error
	)

	number, err = p.consumeNumber()
	if err != nil {
		return 0, err
	}

	unit, err = p.consumeUnit()
	if err != nil {
		return 0, err
	}

	return number * unit, nil
}

func (p *parser) consumeUnit() (int64, error) {
	unit, exist := units[strings.ToLower(strings.TrimSpace(p.r.String()))]
	if !exist {
		return 0, errSyntax
	}
	return unit, nil
}

func (p *parser) consumeNumber() (int64, error) {
	var buf bytes.Buffer
	for {
		b, err := p.r.ReadByte()
		if err != nil {
			break
		}
		if b >= '0' && b <= '9' {
			buf.WriteByte(b)
			continue
		}

		p.r.UnreadByte()
		break
	}
	if buf.Len() == 0 {
		return 1, nil
	}

	number, err := strconv.ParseInt(buf.String(), 10, 64)
	if err != nil {
		return 0, errSyntax
	}
	return number, nil
}
