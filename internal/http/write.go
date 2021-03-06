// Copyright 2014 The Cayley Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"

	"github.com/cayleygraph/cayley/clog"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/internal"
	"github.com/cayleygraph/cayley/quad"
	"github.com/cayleygraph/cayley/quad/nquads"
)

func ParseJSONToQuadList(jsonBody []byte) (out []quad.Quad, _ error) {
	var quads []struct {
		Subject   string `json:"subject"`
		Predicate string `json:"predicate"`
		Object    string `json:"object"`
		Label     string `json:"label"`
	}
	err := json.Unmarshal(jsonBody, &quads)
	if err != nil {
		return nil, err
	}
	out = make([]quad.Quad, 0, len(quads))
	for i, jq := range quads {
		q := quad.Quad{
			Subject:   quad.StringToValue(jq.Subject),
			Predicate: quad.StringToValue(jq.Predicate),
			Object:    quad.StringToValue(jq.Object),
			Label:     quad.StringToValue(jq.Label),
		}
		if !q.IsValid() {
			return nil, fmt.Errorf("invalid quad at index %d. %s", i, q)
		}
		out = append(out, q)
	}
	return out, nil
}

func (api *API) ServeV1Write(w http.ResponseWriter, r *http.Request, _ httprouter.Params) int {
	if api.config.ReadOnly {
		return jsonResponse(w, 400, "Database is read-only.")
	}
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return jsonResponse(w, 400, err)
	}
	quads, err := ParseJSONToQuadList(bodyBytes)
	if err != nil {
		return jsonResponse(w, 400, err)
	}
	h, err := api.GetHandleForRequest(r)
	if err != nil {
		return jsonResponse(w, 400, err)
	}
	if err = h.QuadWriter.AddQuadSet(quads); err != nil {
		return jsonResponse(w, 400, err)
	}
	fmt.Fprintf(w, "{\"result\": \"Successfully wrote %d quads.\"}", len(quads))
	return 200
}

func (api *API) ServeV1WriteNQuad(w http.ResponseWriter, r *http.Request, params httprouter.Params) int {
	if api.config.ReadOnly {
		return jsonResponse(w, 400, "Database is read-only.")
	}

	formFile, _, err := r.FormFile("NQuadFile")
	if err != nil {
		clog.Errorf("%v", err)
		return jsonResponse(w, 500, "Couldn't read file: "+err.Error())
	}
	defer formFile.Close()

	blockSize, blockErr := strconv.Atoi(r.URL.Query().Get("block_size"))
	if blockErr != nil {
		blockSize = api.config.LoadSize
	}

	quadReader, err := internal.Decompressor(formFile)
	// TODO(kortschak) Make this configurable from the web UI.
	dec := nquads.NewReader(quadReader, false)

	h, err := api.GetHandleForRequest(r)
	if err != nil {
		return jsonResponse(w, 400, err)
	}
	n, err := quad.CopyBatch(graph.NewWriter(h), dec, blockSize)
	if err != nil {
		return jsonResponse(w, 400, err)
	}

	fmt.Fprintf(w, "{\"result\": \"Successfully wrote %d quads.\"}", n)
	return 200
}

func (api *API) ServeV1Delete(w http.ResponseWriter, r *http.Request, params httprouter.Params) int {
	if api.config.ReadOnly {
		return jsonResponse(w, 400, "Database is read-only.")
	}
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return jsonResponse(w, 400, err)
	}
	quads, err := ParseJSONToQuadList(bodyBytes)
	if err != nil {
		return jsonResponse(w, 400, err)
	}
	h, err := api.GetHandleForRequest(r)
	if err != nil {
		return jsonResponse(w, 400, err)
	}
	for _, q := range quads {
		err = h.QuadWriter.RemoveQuad(q)
		if err != nil && !graph.IsQuadNotExist(err) {
			return jsonResponse(w, 400, err)
		}
	}
	fmt.Fprintf(w, "{\"result\": \"Successfully deleted %d quads.\"}", len(quads))
	return 200
}
