// Data Use Statement Compliance Checker (DUCK)
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT license.

package carneades

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
	// "path/filepath"
)

// RuleBaseDescription represents a RuleBase Description Yaml file as a struct
type RuleBaseDescription struct {
	Filename    string
	ID          string `yaml:"id"`
	Version     string `yaml:"version"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

// ComplianceCheckerPlugin maps to a ComplianceChecker and RuleBase Descriptions
type ComplianceCheckerPlugin struct {
	checker     *ComplianceChecker
	RuleBaseDir string
	RuleBases   map[string]RuleBaseDescription // RuleBaseDescription.Id -> RuleBaseDescription
}

// MakeComplianceCheckerPlugin returns an error if the ruleBase dir does not
// exist or is not readable.
func MakeComplianceCheckerPlugin(ruleBaseDir string) (*ComplianceCheckerPlugin, error) {
	//	f, e := os.Open(ruleBaseDir)
	//	if e != nil {
	//		return nil, e
	//	}
	i, e := os.Stat(ruleBaseDir)
	if e != nil {
		return nil, e
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("ruleBaseDir %s is not a directory", ruleBaseDir)
	}
	return &ComplianceCheckerPlugin{MakeComplianceChecker(), ruleBaseDir, make(map[string]RuleBaseDescription)}, nil
}

// Intialize For each file in RuleBaseDir:
//    1. Parse the YAML and extract the id, version, title and description
//    2. Call checker.GetTheory function to compile each rulebase into a
//       Carneades theory and cache the theory.
//    3. Create a RuleBaseDescription and add it to the RuleBases map, indexed
//       by its Id.
//  Return an error if any rulebase cannot be compiled into a Theory
func (c *ComplianceCheckerPlugin) Intialize() error {
	files, err := ioutil.ReadDir(c.RuleBaseDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() {
			fr, err := os.Open(filepath.Join(c.RuleBaseDir, file.Name()))
			defer fr.Close()
			if err != nil {
				return err
			}

			//create RuleBaseDescription
			type rb struct {
				Meta RuleBaseDescription
			}
			rby := rb{}
			//read file & Unmarshal content into rby
			dat, err := ioutil.ReadAll(fr)
			if err != nil {
				return err
			}

			err = yaml.Unmarshal(dat, &rby)
			if err != nil {
				return err
			}
			desc := rby.Meta
			desc.Filename = file.Name()
			// compile theory, we dont use it here, it will be cached
			fr, err = os.Open(filepath.Join(c.RuleBaseDir, file.Name()))

			if err != nil {
				return err
			}
			_, err = c.checker.GetTheory(desc.ID, "irrelevant", fr)
			if err != nil {
				return err
			}

			c.RuleBases[desc.ID] = desc

		}
	}

	return nil
}

//Shutdown does nothing yet
func (c ComplianceCheckerPlugin) Shutdown() {
	// Nothing to do
}

// ruleBaseReader returns a reader for reading the JSON source file of the
// rulebase with the given id
func (c *ComplianceCheckerPlugin) ruleBaseReader(ruleBaseID string) io.Reader {
	rb := c.RuleBases[ruleBaseID]
	fr, err := os.Open(filepath.Join(c.RuleBaseDir, rb.Filename))
	if err != nil {
		fmt.Printf("Could not open the rulebase file: %s\n", rb.Filename)
		return nil
	}
	return fr
}

// IsCompliant returns true iff the document complies with the rules in the given
// rulebase.  An error is returned if document has syntax errors and cannot be parsed.
func (c *ComplianceCheckerPlugin) IsCompliant(ruleBaseID string, document *NormalizedDocument) (bool, Explanation, error) {
	r := c.ruleBaseReader(ruleBaseID)
	theory, err := c.checker.GetTheory(ruleBaseID, "irrelevant", r)
	if err != nil {
		return false, nil, err
	}
	return c.checker.IsCompliant(theory, document)
}

// CompliantDocuments returns true iff the document complies with the rules in the given
// rulebase.  An error is returned if document has syntax errors and cannot be parsed. If
// the document is not compliant, false is returned along with a slice of compliant documents
// based on the input document. At most maxResults documents are returned. If offset is greater than
// 0, the first offset compliant documents found are skipped, allowing CompliantDocuments to be
// called repeatedly to scroll through all compliant documents incrementally.  The search
// for compliant documents is restarted each time CompliantDocuments is called, no matter
// what the offset is.
func (c *ComplianceCheckerPlugin) CompliantDocuments(ruleBaseID string, document *NormalizedDocument, maxResults int, offset int) (bool, []*NormalizedDocument, error) {
	fmt.Println("Checking Compliance")
	r := c.ruleBaseReader(ruleBaseID)

	theory, err := c.checker.GetTheory(ruleBaseID, "irrelevant", r)
	if err != nil {
		return false, nil, err
	}

	cncl := MakeCanceller()
	compliant, docChan, err := c.checker.CompliantDocuments(theory, document, cncl)
	if err != nil {
		return false, nil, err
	}
	fmt.Printf("Compliant: %t\n", compliant)
	if compliant {
		return true, nil, nil
	}

	if offset > 0 {
		for k := 0; k < offset; k++ {
			_, ok := <-docChan
			if !ok {
				// no further compliant documents available
				return false, nil, nil
			}
		}
	}
	docs := make([]*NormalizedDocument, 0, maxResults)
	for i := 0; i < maxResults; i++ {
		temp, ok := <-docChan
		if !ok {
			// no further compliant documents available
			fmt.Println("No more compliant documents available")
			return false, docs, nil
		}
		fmt.Printf("Compliant Document Found: %+v \n", temp)
		docs = append(docs, temp)
	}

	cncl.Cancel()
	fmt.Println("Checking Cancelled")
	return false, docs, nil
}
