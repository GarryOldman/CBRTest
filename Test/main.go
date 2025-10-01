package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

type CurrencyRecord struct {
	Date  string
	Code  string
	Name  string
	Value float64
}

type CurrencyAggregator struct {
	Name  string
	Sum   float64
	Count int
}

const (
	CBR_URL        = "http://www.cbr.ru/scripts/XML_daily_eng.asp?date_req=%s"
	REPORT_DAYS    = 90
	API_CALL_DELAY = 100 * time.Millisecond
)

func main() {
	days := REPORT_DAYS
	fmt.Printf("Отчет по курсам валют ЦБ РФ за последние %d дней.\n", days)

	currencyStats := make(map[string]CurrencyAggregator)

	var maxRecord, minRecord CurrencyRecord
	hasRecords := false

	dates := generateDates(days)
	successDays := 0
	totalCurrencyRecords := 0

	for i, dateStr := range dates {
		fmt.Printf("Processing day %d/%d...\n", i+1, len(dates))
		data, err := getCurrencyData(dateStr)
		if err != nil {
			continue
		}

		successDays++
		totalCurrencyRecords += len(data)

		for _, record := range data {
			if stats, exists := currencyStats[record.Code]; exists {
				stats.Sum += record.Value
				stats.Count++
				currencyStats[record.Code] = stats
			} else {
				currencyStats[record.Code] = CurrencyAggregator{
					Name:  record.Name,
					Sum:   record.Value,
					Count: 1,
				}
			}

			if !hasRecords || record.Value > maxRecord.Value {
				maxRecord = record
			}
			if !hasRecords || record.Value < minRecord.Value {
				minRecord = record
			}
			hasRecords = true
		}

		time.Sleep(API_CALL_DELAY)
	}

	if !hasRecords {
		fmt.Println("Не удалось загрузить данные за указанный период.")
		return
	}

	fmt.Printf("Обработано всего %d записей о курсах.\n", totalCurrencyRecords)

	fmt.Printf("Максимальный курс: %.4f руб. за 1 %s (%s)\n",
		maxRecord.Value, maxRecord.Code, maxRecord.Name)
	fmt.Printf("Дата фиксации: %s\n", maxRecord.Date)

	fmt.Printf("Минимальный курс: %.4f руб. за 1 %s (%s)\n",
		minRecord.Value, minRecord.Code, minRecord.Name)
	fmt.Printf("Дата фиксации: %s\n", minRecord.Date)

	fmt.Printf("Количество уникальных валют: %d\n", len(currencyStats))

	fmt.Printf("%-6s %-30s %15s\n", "Код", "Название Валюты", "Средний Курс (руб.)")
	fmt.Println(strings.Repeat("-", 53))

	for code, stats := range currencyStats {
		average := stats.Sum / float64(stats.Count)
		fmt.Printf("%-6s %-30s %15.4f\n", code, stats.Name, average)
	}
}

func getCurrencyData(date string) ([]CurrencyRecord, error) {
	url := fmt.Sprintf(CBR_URL, date)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP запроса к %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неудачный HTTP статус: %d для даты %s", resp.StatusCode, date)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var xmlData struct {
		XMLName xml.Name `xml:"ValCurs"`
		Valutes []struct {
			CharCode string `xml:"CharCode"`
			Nominal  string `xml:"Nominal"`
			Name     string `xml:"Name"`
			Value    string `xml:"Value"`
		} `xml:"Valute"`
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.ToLower(charset) == "windows-1251" {
			return charmap.Windows1251.NewDecoder().Reader(input), nil
		}
		return input, nil
	}

	err = decoder.Decode(&xmlData)
	if err != nil {
		return nil, fmt.Errorf("ошибка декодирования XML: %w", err)
	}

	var records []CurrencyRecord
	for _, v := range xmlData.Valutes {
		value, err := parseCurrencyValue(v.Value)
		if err != nil {
			continue
		}

		nominal, err := strconv.Atoi(v.Nominal)
		if err != nil || nominal == 0 {
			continue
		}

		valuePerOne := value / float64(nominal)

		records = append(records, CurrencyRecord{
			Date:  date,
			Code:  v.CharCode,
			Name:  v.Name,
			Value: valuePerOne,
		})
	}

	return records, nil
}

func parseCurrencyValue(valueStr string) (float64, error) {
	normalized := strings.ReplaceAll(valueStr, ",", ".")
	return strconv.ParseFloat(normalized, 64)
}

func generateDates(days int) []string {
	var dates []string
	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.Add(-time.Duration(i) * 24 * time.Hour)
		dates = append(dates, date.Format("02/01/2006"))
	}
	return dates
}
