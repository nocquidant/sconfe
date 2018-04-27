package main

import (
	"bufio"
	"flag"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

func init() {
	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)
	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)
}

var replacer = strings.NewReplacer("\\", "/")

type env struct {
	dryRun    bool
	configDir string
	inputDir  string
	outputDir string
	profiles  []string
}

func newEnv() env {
	e := env{}

	help := flag.Bool("help", false, "Print this help")
	rootDir := flag.String("rootdir", "/workspace", "Root directory")
	flag.BoolVar(&e.dryRun, "dryrun", false, "Use stdout instead of output path")
	flag.StringVar(&e.configDir, "configdir", "./config", "Config directory relative to root path")
	flag.StringVar(&e.inputDir, "inputdir", "./input", "Input path for files to process relative to root path")
	flag.StringVar(&e.outputDir, "outputdir", "./output", "Output path for processed files relative to root path")
	profiles := flag.String("profiles", "a,b,c", "List of comma separated profiles")
	e.profiles = strings.Split(*profiles, ",")

	flag.Parse()

	if *help {
		flag.PrintDefaults()
		os.Exit(0)
	}

	log.Debugf("Using rootDir=%s", *rootDir)

	// cleanup
	e.configDir = replacer.Replace(path.Clean(path.Join(*rootDir, e.configDir)))
	e.inputDir = replacer.Replace(path.Clean(path.Join(*rootDir, e.inputDir)))
	e.outputDir = replacer.Replace(path.Clean(path.Join(*rootDir, e.outputDir)))

	return e
}

func (e *env) toString() {
	log.Debugf("Parameters are: dryRun=%t, configDir=%s, inputDir=%s, outputDir=%s, profiles=%s", e.dryRun, e.configDir, e.inputDir, e.outputDir, e.profiles)

}

func exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}
	return true
}

func isFile(filePath string) bool {
	if !exists(filePath) {
		return false
	}
	fileInfo, err := os.Stat(filePath)
	if err == nil && fileInfo.Mode().IsRegular() {
		return true
	}
	return false
}

func readConfigFile(filename string) (map[string]string, error) {
	config := make(map[string]string)

	if len(filename) == 0 {
		return config, nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewScanner(file)

	for reader.Scan() {
		line := reader.Text()

		// check if the line has = sign
		// and process the line. Ignore the rest.
		if equal := strings.Index(line, "="); equal >= 0 {
			if key := strings.TrimSpace(line[:equal]); len(key) > 0 {
				value := ""
				if len(line) > equal {
					value = strings.TrimSpace(line[equal+1:])
				}
				// assign the config map
				config[key] = value
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return config, nil
}

func getConfigFiles(e env) []string {
	// default values
	file := e.configDir + "/config.properties"
	if !exists(file) {
		log.WithFields(log.Fields{"file": file}).Fatal("A default file must exist")
	}

	res := make([]string, 1)
	res[0] = file

	// profiles values
	if len(e.profiles) > 0 && e.profiles[0] != "" {
		for i := range e.profiles {
			current := e.configDir + "/config-" + e.profiles[i] + ".properties"
			if exists(current) {
				res = append(res, current)
			}
		}
	}

	return res
}

func buildConfigMap(files []string) (map[string]string, error) {
	res := make(map[string]string)

	for i := range files {
		config, err := readConfigFile(files[i])
		if err != nil {
			return nil, err
		}

		// merge maps
		for k, v := range config {
			res[k] = v
		}
	}

	return res, nil
}

func readWriteFile(e env, config map[string]string, inputPath string) error {
	if !isFile(inputPath) {
		return nil
	}

	inputFile, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	outputPath := e.outputDir + inputPath[len(e.inputDir):]
	outputPath = path.Clean(outputPath)

	os.MkdirAll(path.Dir(outputPath), os.ModePerm)

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	reader := bufio.NewScanner(inputFile)
	writer := bufio.NewWriter(outputFile)
	if e.dryRun {
		writer = bufio.NewWriter(os.Stdout)
	}

	for reader.Scan() {
		line := reader.Text()
		processedLine := line
		for {
			if beginIdx := strings.Index(processedLine, "{{"); beginIdx >= 0 {
				if endIdx := strings.Index(processedLine, "}}"); endIdx >= 0 {
					key := strings.TrimSpace(processedLine[beginIdx+2 : endIdx])
					value := config[key]
					if value == "" {
						log.WithFields(log.Fields{
							"inputPath": inputPath,
							"line":      line,
							"key":       key,
						}).Error("Cannot find value for key")
					}
					processedLine = processedLine[0:beginIdx] + value + processedLine[endIdx+2:len(processedLine)]
					//log.Debug(processedLine)
				} else {
					// Beginning template only?!
					log.WithFields(log.Fields{
						"inputPath": inputPath,
						"line":      line,
					}).Warn("Found malformed template in file")
					break
				}
			} else {
				break // No template
			}
		}
		writer.WriteString(processedLine + "\n")
	}

	return writer.Flush()
}

func processFiles(e env, config map[string]string) error {
	return filepath.Walk(e.inputDir, func(path string, info os.FileInfo, err error) error {
		path = replacer.Replace(path)

		log.WithFields(log.Fields{
			"path": path,
		}).Debug("Visited item")

		return readWriteFile(e, config, path)
	})
}

func main() {
	e := newEnv()
	e.toString() // debug

	files := getConfigFiles(e)
	config, err := buildConfigMap(files)
	if err != nil {
		log.Fatalf("Unexpected error when building config map. %s", err)
	}
	err = processFiles(e, config)
	if err != nil {
		log.Fatalf("Unexpected error when processing files. %s", err)
	}
}
