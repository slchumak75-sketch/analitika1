// Поддержка табличных файлов: .xls (BIFF8/OLE2), .xlsx (OOXML), .csv/.tsv.
// Только стандартная библиотека Go — внешние зависимости по-прежнему не нужны.
//
// Парсер .xls намеренно ограничен практически нужным подмножеством BIFF8:
// записи SST/LABELSST, NUMBER, RK, MULRK, LABEL, FORMULA (кэшированное
// значение). Этого достаточно для отчётов вида «ОТЧЕТ ПО ОРГАНИЗАЦИИ …»
// (Excel 97-2003). Файлы Excel 95 и старше не поддерживаются.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Sheet — один лист таблицы: прямоугольная сетка строк (текст как есть;
// числа хранятся текстом, распарсивание в float делает анализатор).
type Sheet struct {
	Name string     `json:"name"`
	Rows [][]string `json:"rows"`
}

// Workbook — разобранный файл таблицы.
type Workbook struct {
	FileName string  `json:"fileName"`
	Format   string  `json:"format"` // xls | xlsx | csv
	Sheets   []Sheet `json:"sheets"`
}

// ParseSpreadsheet разбирает файл по расширению имени (и сигнатуре).
func ParseSpreadsheet(fileName string, data []byte) (*Workbook, error) {
	lower := strings.ToLower(fileName)
	switch {
	case strings.HasSuffix(lower, ".csv"), strings.HasSuffix(lower, ".tsv"), strings.HasSuffix(lower, ".txt"):
		sh, err := parseCSV(data, strings.HasSuffix(lower, ".tsv"))
		if err != nil {
			return nil, err
		}
		return &Workbook{FileName: fileName, Format: "csv", Sheets: []Sheet{sh}}, nil
	case strings.HasSuffix(lower, ".xlsx"):
		sheets, err := parseXLSX(data)
		if err != nil {
			return nil, err
		}
		return &Workbook{FileName: fileName, Format: "xlsx", Sheets: sheets}, nil
	case strings.HasSuffix(lower, ".xls"):
		sheets, err := parseXLS(data)
		if err != nil {
			return nil, err
		}
		return &Workbook{FileName: fileName, Format: "xls", Sheets: sheets}, nil
	}
	return nil, fmt.Errorf("неподдерживаемый формат: нужны .xls, .xlsx или .csv")
}

// ---------- CSV / TSV ----------

func parseCSV(data []byte, tsv bool) (Sheet, error) {
	text := decodeText(data)
	delim := ','
	if tsv {
		delim = '\t'
	} else if strings.Contains(text, ";") {
		delim = ';'
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil {
		return Sheet{}, fmt.Errorf("csv: %w", err)
	}
	return Sheet{Name: "CSV", Rows: recs}, nil
}

// decodeText: UTF-8 как есть; иначе пробуем windows-1251 (частый случай для RU Excel).
func decodeText(data []byte) string {
	if isValidUTF8(data) {
		return strings.TrimPrefix(string(data), "\ufeff")
	}
	// windows-1251 -> руны: 0x00-0x7F ASCII, 0xC0-0xFF = А-я, остальное по таблице.
	var b strings.Builder
	for _, c := range data {
		switch {
		case c < 0x80:
			b.WriteByte(c)
		case c >= 0xC0:
			b.WriteRune(rune(0x0410) + rune(c-0xC0))
		case c == 0xA8:
			b.WriteRune('Ё')
		case c == 0xB8:
			b.WriteRune('ё')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isValidUTF8(data []byte) bool {
	for i := 0; i < len(data); {
		c := data[i]
		if c < 0x80 {
			i++
			continue
		}
		var size int
		switch {
		case c&0xE0 == 0xC0:
			size = 2
		case c&0xF0 == 0xE0:
			size = 3
		case c&0xF8 == 0xF0:
			size = 4
		default:
			return false
		}
		if i+size > len(data) {
			return false
		}
		for j := 1; j < size; j++ {
			if data[i+j]&0xC0 != 0x80 {
				return false
			}
		}
		i += size
	}
	return true
}

// ---------- XLSX (zip + XML, stdlib) ----------

type xmlSharedStrings struct {
	Items []struct {
		Text string `xml:"t"`
		Rich []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type xmlSheet struct {
	Rows []struct {
		Cells []struct {
			Ref   string `xml:"r,attr"`
			Type  string `xml:"t,attr"`
			Value string `xml:"v"`
			Inl   string `xml:"is>t"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func parseXLSX(data []byte) ([]Sheet, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("xlsx: нет %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	var shared []string
	if raw, err := read("xl/sharedStrings.xml"); err == nil {
		var ss xmlSharedStrings
		if err := xml.Unmarshal(raw, &ss); err == nil {
			for _, it := range ss.Items {
				if it.Text != "" {
					shared = append(shared, it.Text)
				} else {
					var sb strings.Builder
					for _, r := range it.Rich {
						sb.WriteString(r.Text)
					}
					shared = append(shared, sb.String())
				}
			}
		}
	}
	var sheets []Sheet
	for i := 1; ; i++ {
		raw, err := read(fmt.Sprintf("xl/worksheets/sheet%d.xml", i))
		if err != nil {
			break
		}
		var xs xmlSheet
		if err := xml.Unmarshal(raw, &xs); err != nil {
			return nil, fmt.Errorf("xlsx sheet%d: %w", i, err)
		}
		sh := Sheet{Name: fmt.Sprintf("Лист %d", i)}
		for _, row := range xs.Rows {
			var cells []string
			for _, c := range row.Cells {
				col := colFromRef(c.Ref)
				for len(cells) <= col {
					cells = append(cells, "")
				}
				val := c.Value
				if c.Type == "s" {
					if idx, e := strconv.Atoi(val); e == nil && idx >= 0 && idx < len(shared) {
						val = shared[idx]
					}
				} else if c.Type == "inlineStr" {
					val = c.Inl
				}
				cells[col] = val
			}
			sh.Rows = append(sh.Rows, cells)
		}
		sheets = append(sheets, sh)
	}
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx: листы не найдены")
	}
	return sheets, nil
}

func colFromRef(ref string) int {
	col := 0
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if c < 'A' || c > 'Z' {
			break
		}
		col = col*26 + int(c-'A'+1)
	}
	return col - 1
}

// ---------- XLS (OLE2 + BIFF8) ----------

// oleReadStream достаёт поток Workbook из OLE2-контейнера (только обычные
// FAT-секторы; мини-потоки для Workbook не используются — поток большой).
func oleReadStream(data []byte) ([]byte, error) {
	if len(data) < 512 || !bytes.Equal(data[:8], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		return nil, fmt.Errorf("xls: это не OLE2-файл (не Excel 97-2003?)")
	}
	sectorSize := 1 << binary.LittleEndian.Uint16(data[30:32])
	numFAT := int(binary.LittleEndian.Uint32(data[44:48]))
	firstDir := int32(binary.LittleEndian.Uint32(data[48:52]))
	// DIFAT: первые 109 секторов FAT — в заголовке со смещения 76.
	var fatSectors []int32
	for i := 0; i < 109 && len(fatSectors) < numFAT; i++ {
		s := int32(binary.LittleEndian.Uint32(data[76+i*4 : 80+i*4]))
		if s >= 0 {
			fatSectors = append(fatSectors, s)
		}
	}
	sector := func(n int32) []byte {
		off := (int(n) + 1) * sectorSize
		if off < 0 || off+sectorSize > len(data) {
			return nil
		}
		return data[off : off+sectorSize]
	}
	var fat []int32
	for _, fs := range fatSectors {
		sec := sector(fs)
		for i := 0; i+4 <= len(sec); i += 4 {
			fat = append(fat, int32(binary.LittleEndian.Uint32(sec[i:i+4])))
		}
	}
	chain := func(start int32) []byte {
		var out bytes.Buffer
		for s := start; s >= 0 && int(s) < len(fat); {
			out.Write(sector(s))
			next := fat[s]
			if next < 0 {
				break
			}
			s = next
		}
		return out.Bytes()
	}
	dir := chain(firstDir)
	var wbStart int32 = -1
	var wbSize int64
	for off := 0; off+128 <= len(dir); off += 128 {
		e := dir[off : off+128]
		nameLen := int(binary.LittleEndian.Uint16(e[64:66]))
		if nameLen < 2 {
			continue
		}
		nameBytes := e[:nameLen-2]
		runes := make([]rune, 0, nameLen/2)
		for i := 0; i+1 < len(nameBytes); i += 2 {
			runes = append(runes, rune(binary.LittleEndian.Uint16(nameBytes[i:i+2])))
		}
		name := string(runes)
		if name == "Workbook" || name == "Book" {
			wbStart = int32(binary.LittleEndian.Uint32(e[116:120]))
			wbSize = int64(binary.LittleEndian.Uint64(e[120:128]))
			break
		}
	}
	if wbStart < 0 {
		return nil, fmt.Errorf("xls: поток Workbook не найден")
	}
	stream := chain(wbStart)
	if wbSize > 0 && int(wbSize) < len(stream) {
		stream = stream[:wbSize]
	}
	return stream, nil
}

type biffCells map[uint32]map[uint32]string // sheetIdx -> (row<<16|col) -> text

func parseXLS(data []byte) ([]Sheet, error) {
	stream, err := oleReadStream(data)
	if err != nil {
		return nil, err
	}
	// Проход 1: названия листов (BOUNDSHEET) и SST.
	var sheetNames []string
	var sst []string
	for pos := 0; pos+4 <= len(stream); {
		id := binary.LittleEndian.Uint16(stream[pos : pos+2])
		size := int(binary.LittleEndian.Uint16(stream[pos+2 : pos+4]))
		if pos+4+size > len(stream) {
			break
		}
		payload := stream[pos+4 : pos+4+size]
		switch id {
		case 0x0085: // BOUNDSHEET: имя листа (BIFF8, UTF-16LE/ASCII)
			if len(payload) > 7 {
				n := int(payload[6])
				if payload[7]&1 == 1 {
					var sb strings.Builder
					for i := 0; i < n && 8+i*2+1 < len(payload); i++ {
						sb.WriteRune(rune(binary.LittleEndian.Uint16(payload[8+i*2 : 10+i*2])))
					}
					sheetNames = append(sheetNames, sb.String())
				} else if 8+n <= len(payload) {
					sheetNames = append(sheetNames, decodeText(payload[8:8+n]))
				}
			}
		case 0x00FC: // SST (+ CONTINUE следом)
			raw := append([]byte(nil), payload...)
			p2 := pos + 4 + size
			for p2+4 <= len(stream) && binary.LittleEndian.Uint16(stream[p2:p2+2]) == 0x003C {
				sz := int(binary.LittleEndian.Uint16(stream[p2+2 : p2+4]))
				if p2+4+sz > len(stream) {
					break
				}
				raw = append(raw, stream[p2+4:p2+4+sz]...)
				p2 += 4 + sz
			}
			sst = parseSST(raw)
		}
		pos += 4 + size
	}
	// Проход 2: ячейки по листам (листы разделяются BOF с типом 0x0010).
	cells := biffCells{}
	sheetIdx := -1
	set := func(row, col uint32, text string) {
		if sheetIdx < 0 {
			return
		}
		if cells[uint32(sheetIdx)] == nil {
			cells[uint32(sheetIdx)] = map[uint32]string{}
		}
		cells[uint32(sheetIdx)][row<<16|col] = text
	}
	for pos := 0; pos+4 <= len(stream); {
		id := binary.LittleEndian.Uint16(stream[pos : pos+2])
		size := int(binary.LittleEndian.Uint16(stream[pos+2 : pos+4]))
		if pos+4+size > len(stream) {
			break
		}
		p := stream[pos+4 : pos+4+size]
		switch id {
		case 0x0809: // BOF
			if len(p) >= 2 && binary.LittleEndian.Uint16(p[0:2]) == 0x0010 {
				sheetIdx++
			}
		case 0x00FD: // LABELSST
			if len(p) >= 10 {
				row := uint32(binary.LittleEndian.Uint16(p[0:2]))
				col := uint32(binary.LittleEndian.Uint16(p[2:4]))
				idx := int(binary.LittleEndian.Uint32(p[6:10]))
				if idx >= 0 && idx < len(sst) {
					set(row, col, sst[idx])
				}
			}
		case 0x0203: // NUMBER
			if len(p) >= 14 {
				row := uint32(binary.LittleEndian.Uint16(p[0:2]))
				col := uint32(binary.LittleEndian.Uint16(p[2:4]))
				set(row, col, formatFloat(math.Float64frombits(binary.LittleEndian.Uint64(p[6:14]))))
			}
		case 0x027E: // RK
			if len(p) >= 10 {
				row := uint32(binary.LittleEndian.Uint16(p[0:2]))
				col := uint32(binary.LittleEndian.Uint16(p[2:4]))
				set(row, col, formatFloat(decodeRK(binary.LittleEndian.Uint32(p[6:10]))))
			}
		case 0x00BD: // MULRK
			if len(p) >= 6 {
				row := uint32(binary.LittleEndian.Uint16(p[0:2]))
				first := uint32(binary.LittleEndian.Uint16(p[2:4]))
				last := uint32(binary.LittleEndian.Uint16(p[len(p)-2 : len(p)]))
				off := 4
				for c := first; c <= last && off+6 <= len(p); c++ {
					rk := binary.LittleEndian.Uint32(p[off+2 : off+6])
					set(row, c, formatFloat(decodeRK(rk)))
					off += 6
				}
			}
		case 0x0204: // LABEL (BIFF8)
			if len(p) >= 8 {
				row := uint32(binary.LittleEndian.Uint16(p[0:2]))
				col := uint32(binary.LittleEndian.Uint16(p[2:4]))
				n := int(binary.LittleEndian.Uint16(p[6:8]))
				if len(p) > 8 && p[8]&1 == 1 {
					var sb strings.Builder
					for i := 0; i < n && 9+i*2+1 < len(p); i++ {
						sb.WriteRune(rune(binary.LittleEndian.Uint16(p[9+i*2 : 11+i*2])))
					}
					set(row, col, sb.String())
				} else if 8+n <= len(p) {
					set(row, col, decodeText(p[8:8+n]))
				}
			}
		case 0x0006: // FORMULA: первые 8 байт — кэшированный результат
			if len(p) >= 14 {
				bits := binary.LittleEndian.Uint64(p[6:14])
				if bits>>48 != 0xFFFF { // не строка/ошибка
					row := uint32(binary.LittleEndian.Uint16(p[0:2]))
					col := uint32(binary.LittleEndian.Uint16(p[2:4]))
					set(row, col, formatFloat(math.Float64frombits(bits)))
				}
			}
		}
		pos += 4 + size
	}
	if sheetIdx < 0 {
		return nil, fmt.Errorf("xls: листы BIFF не найдены")
	}
	var sheets []Sheet
	for i := 0; i <= sheetIdx; i++ {
		name := fmt.Sprintf("Лист %d", i+1)
		if i < len(sheetNames) {
			name = sheetNames[i]
		}
		sheets = append(sheets, Sheet{Name: name, Rows: cellsToRows(cells[uint32(i)])})
	}
	return sheets, nil
}

func cellsToRows(cells map[uint32]string) [][]string {
	maxR, maxC := 0, 0
	for k := range cells {
		r, c := int(k>>16), int(k&0xFFFF)
		if r > maxR {
			maxR = r
		}
		if c > maxC {
			maxC = c
		}
	}
	rows := make([][]string, maxR+1)
	for i := range rows {
		rows[i] = make([]string, maxC+1)
	}
	for k, v := range cells {
		rows[k>>16][k&0xFFFF] = v
	}
	return rows
}

// parseSST разбирает Shared String Table BIFF8 (без сложных rich-text/continue-границ:
// продолжения уже склеены вызывающим кодом; строки, разрезанные по границе
// CONTINUE внутри символов, могут потерять флаг кодировки — допустимое упрощение).
func parseSST(p []byte) []string {
	if len(p) < 8 {
		return nil
	}
	unique := int(binary.LittleEndian.Uint32(p[4:8]))
	pos := 8
	var out []string
	for len(out) < unique && pos < len(p) {
		if pos+2 > len(p) {
			break
		}
		n := int(binary.LittleEndian.Uint16(p[pos : pos+2]))
		pos += 2
		if pos >= len(p) {
			break
		}
		flags := p[pos]
		pos++
		is16 := flags&1 == 1
		isExt := flags&8 != 0
		isRich := flags&4 != 0
		var richRuns uint16
		var extSize uint32
		if isRich && pos+2 <= len(p) {
			richRuns = binary.LittleEndian.Uint16(p[pos : pos+2])
			pos += 2
		}
		if isExt && pos+4 <= len(p) {
			extSize = binary.LittleEndian.Uint32(p[pos : pos+4])
			pos += 4
		}
		var sb strings.Builder
		for i := 0; i < n; i++ {
			if is16 {
				if pos+2 > len(p) {
					break
				}
				sb.WriteRune(rune(binary.LittleEndian.Uint16(p[pos : pos+2])))
				pos += 2
			} else {
				if pos >= len(p) {
					break
				}
				sb.WriteString(decodeText(p[pos : pos+1]))
				pos++
			}
		}
		pos += int(richRuns)*4 + int(extSize)
		out = append(out, sb.String())
	}
	return out
}

func decodeRK(rk uint32) float64 {
	div100 := rk&1 != 0
	isInt := rk&2 != 0
	var v float64
	if isInt {
		v = float64(int32(rk) >> 2)
	} else {
		bits := uint64(rk&0xFFFFFFFC) << 32
		v = math.Float64frombits(bits)
	}
	if div100 {
		v /= 100
	}
	return v
}

func formatFloat(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// parseNumberCell разбирает числовую ячейку ("1 234,56", "12%", числа float).
func parseNumberCell(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
