/*
 * Copyright © 2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @author		Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @Copyright 	2017-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @license 	Apache-2.0
 */

package driver_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ory/viper"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ory/fosite"
	. "github.com/ory/hydra/driver"
	"github.com/ory/hydra/internal"
	"github.com/ory/hydra/oauth2"
	"github.com/ory/hydra/x"

	"github.com/ory/hydra/client"
)

func TestOAuth2AwareCORSMiddleware(t *testing.T) {
	c := internal.NewConfigurationWithDefaults()
	r := internal.NewRegistryMemory(t, c)

	token, signature, _ := r.OAuth2HMACStrategy().GenerateAccessToken(nil, nil)
	for k, tc := range []struct {
		prep         func()
		d            string
		mw           func(http.Handler) http.Handler
		code         int
		header       http.Header
		expectHeader http.Header
	}{
		{
			d:            "should ignore when disabled",
			prep:         func() {},
			code:         http.StatusNotImplemented,
			header:       http.Header{},
			expectHeader: http.Header{},
		},
		{
			d: "should reject when basic auth but client does not exist and cors enabled",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo", "bar"))}},
			expectHeader: http.Header{"Vary": {"Origin"}},
		},
		{
			d: "should reject when basic auth client exists but origin not allowed",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-2", Secret: "bar", AllowedCORSOrigins: []string{"http://not-foobar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-2", "bar"))}},
			expectHeader: http.Header{"Vary": {"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin allowed",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-3", Secret: "bar", AllowedCORSOrigins: []string{"http://foobar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-3", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with partial wildcard) is allowed per client",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-4", Secret: "bar", AllowedCORSOrigins: []string{"http://*.foobar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foo.foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-4", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foo.foobar.com"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with full wildcard) is allowed globally",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"*"})
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-5", Secret: "bar", AllowedCORSOrigins: []string{"http://barbar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"*"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-5", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"*"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with partial wildcard) is allowed globally",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://*.foobar.com"})
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-6", Secret: "bar", AllowedCORSOrigins: []string{"http://barbar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foo.foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-6", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foo.foobar.com"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with full wildcard) allowed per client",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-7", Secret: "bar", AllowedCORSOrigins: []string{"*"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-7", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should fail when token introspection fails",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {"Bearer 1234"}},
			expectHeader: http.Header{"Vary": {"Origin"}},
		},
		{
			d: "should work when token introspection returns a session",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
				sess := oauth2.NewSession("foo-9")
				sess.SetExpiresAt(fosite.AccessToken, time.Now().Add(time.Hour))
				ar := fosite.NewAccessRequest(sess)
				cl := &client.Client{ID: "foo-9", Secret: "bar", AllowedCORSOrigins: []string{"http://foobar.com"}}
				ar.Client = cl
				if err := r.ClientManager().CreateClient(context.Background(), cl); err != nil {
					panic(err)
				}
				if err := r.OAuth2Storage().CreateAccessTokenSession(context.Background(), signature, ar); err != nil {
					panic(err)
				}
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {"Bearer " + token}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept any allowed specified origin protocol",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-11", Secret: "bar", AllowedCORSOrigins: []string{"*"}})
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://*", "https://*"})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foo.foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-11", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foo.foobar.com"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept client origin when basic auth client exists and origin is set at the client as well as the server",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://**.example.com"})
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-12", Secret: "bar", AllowedCORSOrigins: []string{"http://myapp.example.biz"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://myapp.example.biz"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-12", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://myapp.example.biz"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept server origin when basic auth client exists and origin is set at the client as well as the server",
			prep: func() {
				viper.Set("serve.public.cors.enabled", true)
				viper.Set("serve.public.cors.allowed_origins", []string{"http://**.example.com"})
				r.ClientManager().CreateClient(context.Background(), &client.Client{ID: "foo-13", Secret: "bar", AllowedCORSOrigins: []string{"http://myapp.example.biz"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://client-app.example.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-13", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://client-app.example.com"}, "Access-Control-Expose-Headers": []string{"Content-Type"}, "Vary": []string{"Origin"}},
		},
	} {
		t.Run(fmt.Sprintf("case=%d/description=%s", k, tc.d), func(t *testing.T) {
			if tc.prep != nil {
				viper.Reset()
				tc.prep()
			}

			req, err := http.NewRequest("GET", "http://foobar.com/", nil)
			require.NoError(t, err)
			for k := range tc.header {
				req.Header.Set(k, tc.header.Get(k))
			}

			res := httptest.NewRecorder()
			OAuth2AwareCORSMiddleware("public", r, c)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
			})).ServeHTTP(res, req)
			require.NoError(t, err)
			assert.EqualValues(t, tc.code, res.Code)
			assert.EqualValues(t, tc.expectHeader, res.Header())
		})
	}

}
