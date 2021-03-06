// Data Use Statement Compliance Checker (DUCK)
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT license.

package carneades

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/carneades/carneades-4/src/engine/caes"
	"github.com/carneades/carneades-4/src/engine/caes/encoding/graphml"
	y "github.com/carneades/carneades-4/src/engine/caes/encoding/yaml"
)

// DEBUG is set to true to print debug information and save a graphml file of the graph in the temp folder
const DEBUG = false

type Canceller chan struct{}

func MakeCanceller() Canceller {
	return make(chan struct{})
}

func (c Canceller) Cancel() {
	close(c)
}

type VersionedTheory struct {
	revision string
	theory   *caes.Theory
}

type ComplianceChecker struct {
	Theories map[string]VersionedTheory
}

func MakeComplianceChecker() *ComplianceChecker {
	return &ComplianceChecker{make(map[string]VersionedTheory)}
}

// GetTheory retrieves the theory for the given ruleBaseID. If no version of the
// rulebase has been compiled or its revision is not equal to the revision
// used to compile the theory,
// the theory is first updated, by reading the JSON source from rbSrc,
// and compiling the rulebase into a theory and updating the Theories of the
// ComplianceChecker.
// If there are no errors, the returned error will be nil.
func (c ComplianceChecker) GetTheory(ruleBaseID string, revision string, rbSrc io.Reader) (*caes.Theory, error) {

	vt, found := c.Theories[ruleBaseID]
	if !found || revision != vt.revision {

		// Compile the rulebase, update the theory cache and return the
		// theory.  Or return an error if the rulebase cannot be compiled.
		ag, err := y.Import(rbSrc)
		if err != nil {
			fmt.Printf("Could not parse the rulebase\n")
			return nil, err
		}
		log.Printf("rulebase successfully imported, with %d schemes and %d predicates\n", len(ag.Theory.ArgSchemes), len(ag.Theory.Language))
		log.Printf("title: %s\n", ag.Metadata["title"].(string))
		c.Theories[ruleBaseID] = VersionedTheory{revision, ag.Theory}
		return ag.Theory, nil
	}
	fmt.Printf("rulebase successfully imported\n")
	return vt.theory, nil
}

/*
	IsCompliant does the following:
		* Translates the data use statements in the document into Carneades assumptions (terms)
		* Applies the theory to the assumptions, using the Carneades inference engine,
		    to construct a Carneades argument graph
	    * Evaluates the argument graph to label the statements in the graph in, out or undecided.
		* Returns true if and only if the statement in the argument representing
		    the proposition that the document does not require consent is in.
	Whether or not the document is compliant, an explanation is returned.
	If the document is not compliant, i.e. requires consent, the explanation
	lists the statements which require consent.
	If the document is compliant, i.e. does not require consent, the explanation
	lists assumptions which were made to reach this conclusion, i.e. whether it
	was assumed that a statement is not about pii and whether it was assumed
	that there is a legitimate interest in the content information.
	The error returned will be nil if and only if no errors occur this process.
*/

func (c ComplianceChecker) IsCompliant(theory *caes.Theory, document *NormalizedDocument) (bool, Explanation, error) {
	// func (c ComplianceChecker) IsCompliant(theory *caes.Theory, document *NormalizedDocument) (bool, error) {
	// Construct the argument graph
	ag := caes.NewArgGraph()
	ag.Theory = theory
	// add statements for the data use statements in the document
	// to the argument graph, and assume them to be true.
	for _, s := range document.Statements {
		var passive bool
		if s.Passive {
			passive = true
		} else {
			passive = false
		}

		stmtID := fmt.Sprintf("dataUseStatement(dus(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%t))",
			s.UseScopeCode,
			s.UseScopeLocation,
			s.QualifierCode,
			s.DataCategoryCode,
			s.SourceScopeCode,
			s.SourceScopeLocation,
			s.ActionCode,
			s.ResultScopeCode,
			s.ResultScopeLocation,
			s.TrackingID,
			s.PlaceInStruct,
			passive)

		stmt := &caes.Statement{
			Id:       stmtID,
			Metadata: make(map[string]interface{}),
			Text:     stmtID,
			Args:     []*caes.Argument{}}
		ag.Assumptions = append(ag.Assumptions, stmtID)
		ag.Statements[stmtID] = stmt

	}
	// add statements for the is a relationships in the document
	// to the argument graph, and assume them to be true.

	for k, v := range document.IsA {
		isa := fmt.Sprintf("isA(%s,%s)", k, v)
		if DEBUG {
			log.Println(isa)
		}

		stmt := &caes.Statement{
			Id:       isa,
			Metadata: make(map[string]interface{}),
			Text:     isa,
			Args:     []*caes.Argument{}}
		ag.Assumptions = append(ag.Assumptions, isa)
		ag.Statements[isa] = stmt
	}
	if DEBUG && (document.IsA == nil || len(document.IsA) == 0) {
		log.Println("IsA is empty.")
	}
	// derive arguments by applying the theory of the argument graph to
	// its assumptions
	err := ag.Infer()
	if err != nil {
		return false, nil, err
	}

	// evaluate the argument graph
	l := ag.GroundedLabelling()
	ag.ApplyLabelling(l)

	// write the argument graph in dot to a temporary file
	// so that it can be visualized for debugging purposes

	if DEBUG {
		f, err := ioutil.TempFile(os.TempDir(), "duckGraphml")
		if err == nil {
			graphml.Export(f, ag)
		}
	}

	// return true iff the notDocConsentRequired statement is in
	s, ok := ag.Statements["notDocConsentRequired"]
	if !ok {
		return false, nil, errors.New("notDocConsentRequired is not a statement in the argument graph")
	}
	e, err := c.GetExplanation(theory, ag)
	if err != nil {
		return false, nil, err
	}

	return s.Label == caes.In, e, nil
}

func removeStatement(d *NormalizedDocument, i int) (*NormalizedDocument, error) {
	if i < 0 || i > len(d.Statements)-1 {
		return nil, fmt.Errorf("Statements index out of bounds: %v", i)
	}
	// copy the document struct
	d2 := *d
	// replace the statements with a copy, but with the selected
	// statement removed.
	d2.Statements = []NormalizedStatement{}
	for j := range d.Statements {
		if i != j {
			d2.Statements = append(d2.Statements, d.Statements[j])
		}
	}
	return &d2, nil
}

//CompliantDocuments does the following:
/*	* Translates the data use statements in the document into Carneades assumptions (terms)
	* Applies the theory to the assumptions, using the Carneades inference engine,
	  to construct a Carneades argument graph
    * Evaluates the argument graph to label the statements in the graph in, out or undecided.
	* Starts a coroutine to search for compliant data use documents and returns a channel of pointers
	  to the compliant documents found. If the input document is compliant, the bool result will be true
	  and the channel returned will be closed. If the input document is not
	  compliant, the bool result will be false, and compliant alternative documents based in
	  input document will returned in the channel. The documents returend will have
	  minimal changes sufficient to achieve compliance. The input document is not modified.
	  The coroutine closes the channel when it has finished the search for compliant documents.
An error will be returned only if was not possible to check the compliance of the input document,
before starting the coroutine to search for compliant alternatives.
The caller must bind c to a newly constructed Canceller, with MakeCanceller().
If no error is returned (i.e. error is nil) the caller should call c.Cancel() when no further
documents are needed, to cause the coroutine to be terminated.
*/
func (c ComplianceChecker) CompliantDocuments(theory *caes.Theory, doc *NormalizedDocument, cncl Canceller) (bool, <-chan *NormalizedDocument, error) {
	compliant, _, err := c.IsCompliant(theory, doc)
	if err != nil {
		return false, nil, err
	}

	if compliant {
		return true, nil, nil
	}

	// Traverse the space of documents, breadth-first, with subsets of the data use
	// statements of doc, and push each alternative documents down the
	// variants channel.

	variants := make(chan *NormalizedDocument)

	// generate documents with subsets of the data use documents
	// and push them down the varients channel
	var subsets func(int, *NormalizedDocument)
	subsets = func(i int, d *NormalizedDocument) {
		if i < 0 {
			return
		}
		select {
		case <-cncl:
			return // cancelled
		default: // continue
		}
		// ToDo: check that the statement order below
		// performs breadth-first search, not depth-first
		d2, err := removeStatement(d, i)
		if err != nil {
			return
		} // indexing error should not happen
		fmt.Printf(".") // debugging
		variants <- d2
		subsets(i-1, d)
		subsets(i-1, d2)
	}
	go func() {
		subsets(len(doc.Statements)-1, doc)
		close(variants)
	}()

	// Filter out the noncompliant documents
	compliantVariants := make(chan *NormalizedDocument)

	go func() {
		for {
			select {
			case <-cncl:
				return // cancelled
			case d3, ok := <-variants:
				if !ok { // no more variants available
					close(compliantVariants)
					return
				}

				compliant, _, err := c.IsCompliant(theory, d3)
				if err != nil {
					fmt.Printf("Compliance checking error: %v\n", err)
				} else if compliant {
					compliantVariants <- d3
					fmt.Printf("+%d", len(d3.Statements))
				} else {
					fmt.Printf("-%d", len(d3.Statements))
				}
			default: // do nothing, to avoid blocking
			}
		}
	}()

	return false, compliantVariants, nil
}
