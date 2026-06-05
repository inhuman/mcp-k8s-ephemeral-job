package config

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/inhuman/cleanenv"
	"github.com/sanity-io/litter"
	"github.com/showa-93/go-mask"
)


type Descriptioner interface {
	Description() (string, error)
}

type OnloadPrinter interface {
	Print(value any)
}

func Description(descs ...Descriptioner) (string, error) {
	var fullDescription string
	for _, desc := range descs {
		description, err := desc.Description()
		if err != nil {
			//nolint:wrapcheck
			return "", err
		}
		fullDescription += description + "\n\n"
	}
	return fullDescription, nil
}

type loadOptions struct {
	filePath      string
	searchInFile  bool
	onLoadPrint   bool
	onloadPrinter OnloadPrinter
}

func defaultOptions() loadOptions {
	return loadOptions{
		filePath:     ".env",
		searchInFile: false,
	}
}

type Option func(o *loadOptions)

func WithSearchInDotEnvFile() Option {
	return func(o *loadOptions) {
		o.searchInFile = true
	}
}

func WithPrintOnLoad() Option {
	return func(o *loadOptions) {
		o.onLoadPrint = true
	}
}

func WithOnLoadPrinter(printer OnloadPrinter) Option {
	return func(o *loadOptions) {
		o.onloadPrinter = printer
	}
}

func WithFilePath(path string) Option {
	return func(o *loadOptions) {
		o.filePath = path
	}
}

func Load(confStructPtr any, opts ...Option) error {
	options := defaultOptions()
	for _, o := range opts {
		o(&options)
	}

	if options.onLoadPrint {
		defer func() {
			var printer OnloadPrinter = defaultPrinter{}
			if options.onloadPrinter != nil {
				printer = options.onloadPrinter
			}
			masked, err := mask.Mask(confStructPtr)
			if err != nil {
				printer.Print(err)
				return
			}
			printer.Print(masked)
		}()
	}

	if options.searchInFile && options.filePath != "" {
		if err := cleanenv.ReadConfig(options.filePath, confStructPtr); err == nil {
			return nil
		}
	}

	//nolint:wrapcheck
	return cleanenv.ReadEnv(confStructPtr)
}

func Marshal(confStructPtr any, marshaller func(v any) ([]byte, error)) ([]byte, error) {
	masked, err := mask.Mask(confStructPtr)
	if err != nil {
		//nolint:wrapcheck
		return nil, err
	}
	return marshaller(masked)
}

func JSONMarshalIntend(v any) ([]byte, error) {
	//nolint:wrapcheck
	return json.MarshalIndent(v, " ", "  ")
}

type defaultPrinter struct{}

func (d defaultPrinter) Print(value any) {
	//nolint:forbidigo
	fmt.Println(litter.Sdump(value))
}

type JSONPrinter struct{}

func (j JSONPrinter) Print(value any) {
	jsonBytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(string(jsonBytes))
}
