package main

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <openapi-file>", os.Args[0])
	}
	filename := os.Args[1]

	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		log.Fatalf("Error unmarshaling YAML: %v", err)
	}

	// 1. Find the "paths" node and "components/schemas" node
	pathsNode := findNode(&root, "paths")
	if pathsNode == nil {
		log.Fatalf("Could not find 'paths' in OpenAPI spec")
	}

	componentsNode := findNode(&root, "components")
	if componentsNode == nil {
		log.Fatalf("Could not find 'components' in OpenAPI spec")
	}
	schemasNode := findNode(componentsNode, "schemas")
	if schemasNode == nil {
		log.Fatalf("Could not find 'schemas' in 'components'")
	}

	// 1.5 Fix duplicated operationId for POST /api/v3/traces
	// Iterate paths to find /api/v3/traces -> post -> operationId
	for i := 0; i < len(pathsNode.Content); i += 2 {
		pathKey := pathsNode.Content[i].Value
		if pathKey == "/api/v3/traces" {
			pathVal := pathsNode.Content[i+1]
			// Find "post"
			for j := 0; j < len(pathVal.Content); j += 2 {
				method := pathVal.Content[j].Value
				if method == "post" {
					methodVal := pathVal.Content[j+1]
					// Find "operationId"
					for k := 0; k < len(methodVal.Content); k += 2 {
						if methodVal.Content[k].Value == "operationId" {
							if methodVal.Content[k+1].Value == "QueryService_FindTraces" {
								methodVal.Content[k+1].Value = "QueryService_FindTracesPost"
							}
						}
					}
				}
			}
		}
	}

	// 2. Identify all reachable schemas starting from "paths"
	reachable := make(map[string]bool)

	// Start traversal from root to find initial references
	queue := make([]string, 0)
	initialRefFinder := func(n *yaml.Node) {
		findRefs(n, &reachable, &queue)
	}

	// Traverse root structure, intentionally skipping components/schemas definition scan
	if root.Kind == yaml.DocumentNode {
		rootMap := root.Content[0]
		for i := 0; i < len(rootMap.Content); i += 2 {
			key := rootMap.Content[i].Value
			val := rootMap.Content[i+1]

			if key == "components" {
				for j := 0; j < len(val.Content); j += 2 {
					compKey := val.Content[j].Value
					compVal := val.Content[j+1]
					if compKey == "schemas" {
						continue
					}
					traverse(compVal, initialRefFinder)
				}
			} else {
				traverse(val, initialRefFinder)
			}
		}
	}

	// Now process queue
	// For each schema in queue, valid it exists in schemasNode, then traverse IT to find more refs
	schemaMap := make(map[string]*yaml.Node)
	for i := 0; i < len(schemasNode.Content); i += 2 {
		schemaMap[schemasNode.Content[i].Value] = schemasNode.Content[i+1]
	}

	processed := make(map[string]bool)

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if processed[name] {
			continue
		}
		processed[name] = true

		node, exists := schemaMap[name]
		if !exists {
			// Ref points to non-existent schema? Ignore or warn.
			log.Printf("Warning: reference to non-existent schema %q; ignoring", name)
			continue
		}

		// Traverse this schema definition to find downstream refs
		traverse(node, initialRefFinder)
	}

	// 3. Prune schemas
	// Rebuild schemasNode.Content keeping only those in 'reachable'
	newContent := make([]*yaml.Node, 0)
	for i := 0; i < len(schemasNode.Content); i += 2 {
		name := schemasNode.Content[i].Value
		val := schemasNode.Content[i+1]

		if reachable[name] {
			newContent = append(newContent, schemasNode.Content[i], val)
		} else {
			fmt.Printf("Pruning unused schema: %s\n", name)
		}
	}
	schemasNode.Content = newContent

	// 4. Write back
	outFile, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer outFile.Close()

	encoder := yaml.NewEncoder(outFile)
	encoder.SetIndent(4)
	if err := encoder.Encode(&root); err != nil {
		log.Fatalf("Error encoding YAML: %v", err)
	}
}

func traverse(node *yaml.Node, visitor func(*yaml.Node)) {
	visitor(node)
	for _, child := range node.Content {
		traverse(child, visitor)
	}
}

func findRefs(node *yaml.Node, reachable *map[string]bool, queue *[]string) {
	// Check if current node is a ref
	// A ref in yaml.v3 is somewhat structural.
	// Usually it looks like Key: $ref, Value: "#/components/schemas/Foo" in a map
	// OR simply scanning scalar values checking for prefix.

	// Standard $ref is a key-value pair in a mapping.
	// However, the traverse function visits all nodes.
	// If we are at a Scalar node with Value starting with "#/components/schemas/", it's likely a value of a $ref key.
	// But to be precise we should check parent?
	// Actually, scanning all scalars for the string pattern is safe enough for this specific domain.

	if node.Kind == yaml.ScalarNode {
		val := node.Value
		prefix := "#/components/schemas/"
		if len(val) > len(prefix) && val[:len(prefix)] == prefix {
			schemaName := val[len(prefix):]
			if !(*reachable)[schemaName] {
				(*reachable)[schemaName] = true
				*queue = append(*queue, schemaName)
			}
		}
	}
}

func findNode(root *yaml.Node, key string) *yaml.Node {
	// Assuming root is Document -> Mapping
	var mapNode *yaml.Node
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) > 0 {
			mapNode = root.Content[0]
		}
	} else {
		mapNode = root
	}

	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}
