package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pb33f/libopenapi/index"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"gopkg.in/yaml.v3"
)

// Função para converter para UTF-8
func convertToUTF8(data []byte) ([]byte, error) {
	utf8Bom := unicode.BOMOverride(transform.Nop) // Remove BOM se existir
	reader := transform.NewReader(bytes.NewReader(data), utf8Bom)
	convertedData, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter encoding para UTF-8: %v", err)
	}
	return convertedData, nil
}

// Função para ler um arquivo local, converter para UTF-8 e retornar os bytes
func readFile(filePath string) ([]byte, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler o arquivo %s: %v", filePath, err)
	}

	// Converte para UTF-8 antes de processar
	utf8Data, err := convertToUTF8(data)
	if err != nil {
		return nil, err
	}

	return utf8Data, nil
}

// Função para carregar as regras personalizadas do pb33f_rules.yaml
func loadRules(ruleFile string) (map[string]interface{}, error) {
	data, err := ioutil.ReadFile(ruleFile)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler regras YAML: %v", err)
	}

	var rules map[string]interface{}
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("erro ao fazer unmarshal das regras: %v", err)
	}

	return rules, nil
}

// Função para validar um arquivo OpenAPI usando regras personalizadas
func validateOpenAPIWithRules(filePath string, rulesFile string) error {
	// Ler o arquivo OpenAPI e converter para UTF-8
	data, err := readFile(filePath)
	if err != nil {
		return err
	}

	// Criar um nó YAML a partir do arquivo
	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return fmt.Errorf("erro ao fazer unmarshal do YAML: %v", err)
	}

	// Criar configuração de indexação
	indexConfig := index.CreateClosedAPIIndexConfig()

	// Criar um indexador para a especificação OpenAPI
	idx := index.NewSpecIndex(&rootNode, indexConfig)

	// Obter erros básicos do OpenAPI
	validationErrors := idx.GetValidationErrors()

	// Carregar regras personalizadas
	rules, err := loadRules(rulesFile)
	if err != nil {
		return err
	}

	// Aplicar regras personalizadas
	if rulesMap, ok := rules["rules"].(map[string]interface{}); ok {
		for ruleName, rule := range rulesMap {
			ruleData := rule.(map[string]interface{})
			given := ruleData["given"].(string)
			severity := ruleData["severity"].(string)

			// Aplicação manual de regras
			if given == "$.components.securitySchemes" {
				if idx.GetSecuritySchemes() == nil {
					validationErrors = append(validationErrors, fmt.Errorf("[%s] %s", severity, ruleData["description"]))
				}
			}

			if given == "$.info.contact" {
				if idx.GetContactInfo() == nil {
					validationErrors = append(validationErrors, fmt.Errorf("[%s] %s", severity, ruleData["description"]))
				}
			}

			if given == "$.servers[*].url" {
				servers := idx.GetAllServerURLs()
				for _, url := range servers {
					if !bytes.HasPrefix([]byte(url), []byte("https://")) {
						validationErrors = append(validationErrors, fmt.Errorf("[%s] %s", severity, ruleData["description"]))
					}
				}
			}
		}
	}

	// Exibir erros encontrados
	if len(validationErrors) > 0 {
		for _, err := range validationErrors {
			fmt.Println("❌ Erro de validação:", err)
		}
		return fmt.Errorf("falha na validação do OpenAPI")
	}

	fmt.Println("✅ OpenAPI válido com regras aplicadas:", filePath)
	return nil
}

// Função para resolver referências OpenAPI e salvar o arquivo resolvido
func resolveOpenAPI(inputFile, outputFile string) error {
	// Ler o arquivo e converter para UTF-8
	data, err := readFile(inputFile)
	if err != nil {
		return err
	}

	// Criar um nó YAML a partir do arquivo
	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return fmt.Errorf("erro ao fazer unmarshal do YAML: %v", err)
	}

	// Criar uma configuração para o indexador
	indexConfig := index.CreateClosedAPIIndexConfig()

	// Criar um novo rolodex para gerenciar referências
	rolodex := index.NewRolodex(indexConfig)

	// Definir o root node do rolodex
	rolodex.SetRootNode(&rootNode)

	// Indexar as referências do OpenAPI
	if err := rolodex.IndexTheRolodex(); err != nil {
		return fmt.Errorf("erro ao indexar as referências: %v", err)
	}

	// Resolver todas as referências
	rolodex.Resolve()

	// Criar um YAML resolvido a partir do rolodex atualizado
	resolvedYAML, err := yaml.Marshal(&rootNode)
	if err != nil {
		return fmt.Errorf("erro ao converter para YAML: %v", err)
	}

	// Salvar o YAML resolvido em um novo arquivo
	if err := ioutil.WriteFile(outputFile, resolvedYAML, 0644); err != nil {
		return fmt.Errorf("erro ao salvar arquivo resolvido: %v", err)
	}

	fmt.Println("✅ Arquivo resolvido salvo em:", outputFile)
	return nil
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Uso: go run main.go oldSwagger.yaml swagger.yaml pb33f_rules.yaml")
		return
	}

	oldFile := os.Args[1]
	newFile := os.Args[2]
	rulesFile := os.Args[3]

	// Validar arquivos com regras personalizadas antes de resolver
	if err := validateOpenAPIWithRules(oldFile, rulesFile); err != nil {
		fmt.Println("❌ OpenAPI inválido:", oldFile)
		os.Exit(1)
	}

	if err := validateOpenAPIWithRules(newFile, rulesFile); err != nil {
		fmt.Println("❌ OpenAPI inválido:", newFile)
		os.Exit(1)
	}

	// Resolver e salvar os arquivos
	if err := resolveOpenAPI(oldFile, "oldSwaggerResolve.yaml"); err != nil {
		fmt.Println("❌ Erro ao processar oldSwagger.yaml:", err)
		os.Exit(1)
	}

	if err := resolveOpenAPI(newFile, "swaggerResolve.yaml"); err != nil {
		fmt.Println("❌ Erro ao processar swagger.yaml:", err)
		os.Exit(1)
	}

	fmt.Println("🚀 OpenAPI validado com regras aplicadas e arquivos resolvidos gerados com sucesso!")
}
