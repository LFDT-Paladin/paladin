/*
 * Copyright contributors to Paladin, an LFDT project
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package org.lfdt.paladin.sdk.client.rpc;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;

class JsonRpcEnvelopeTest {

    private final ObjectMapper mapper = new ObjectMapper();

    @Test
    void requestSerializesWithDefaultVersionAndParams() throws Exception {
        JsonRpcRequest req = new JsonRpcRequest("000000001", "ptx_sendTransaction", List.of("a", 42));
        JsonNode json = mapper.readTree(mapper.writeValueAsBytes(req));
        assertEquals("2.0", json.get("jsonrpc").asText());
        assertEquals("000000001", json.get("id").asText());
        assertEquals("ptx_sendTransaction", json.get("method").asText());
        assertTrue(json.get("params").isArray());
        assertEquals(2, json.get("params").size());
        assertEquals("a", json.get("params").get(0).asText());
        assertEquals(42, json.get("params").get(1).asInt());
    }

    @Test
    void requestOmitsEmptyParams() throws Exception {
        JsonRpcRequest req = new JsonRpcRequest("000000002", "pstate_listSchemas", List.of());
        JsonNode json = mapper.readTree(mapper.writeValueAsBytes(req));
        assertFalse(json.has("params"), "empty params must be omitted to mirror Go omitempty");
    }

    @Test
    void nullParamsBecomeEmptyAndAreOmitted() throws Exception {
        JsonRpcRequest req = new JsonRpcRequest("000000003", "noop", null);
        assertTrue(req.params().isEmpty());
        JsonNode json = mapper.readTree(mapper.writeValueAsBytes(req));
        assertFalse(json.has("params"));
    }

    @Test
    void successResponseExposesRawResultAndNoError() throws Exception {
        String body = "{\"jsonrpc\":\"2.0\",\"id\":\"000000001\",\"result\":{\"value\":\"0x1f\"}}";
        JsonRpcResponse resp = mapper.readValue(body, JsonRpcResponse.class);
        assertFalse(resp.hasError());
        assertNull(resp.error());
        assertEquals("0x1f", resp.result().get("value").asText());
        assertEquals("000000001", resp.id().asText());
    }

    @Test
    void numericResponseIdParses() throws Exception {
        String body = "{\"jsonrpc\":\"2.0\",\"id\":7,\"result\":\"ok\"}";
        JsonRpcResponse resp = mapper.readValue(body, JsonRpcResponse.class);
        assertEquals(7, resp.id().asInt());
        assertEquals("ok", resp.result().asText());
    }

    @Test
    void errorResponseExposesCodeMessageAndData() throws Exception {
        String body =
                "{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"error\":{\"code\":-32000,\"message\":\"unauthorized\",\"data\":{\"hint\":\"token\"}}}";
        JsonRpcResponse resp = mapper.readValue(body, JsonRpcResponse.class);
        assertTrue(resp.hasError());
        assertNull(resp.result());
        JsonRpcError err = resp.error();
        assertEquals(JsonRpcErrorCode.UNAUTHORIZED, err.code());
        assertEquals("unauthorized", err.message());
        assertEquals("token", err.data().get("hint").asText());
    }

    @Test
    void errorWithoutDataReportsNull() throws Exception {
        String body = "{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"error\":{\"code\":-32603,\"message\":\"boom\"}}";
        JsonRpcResponse resp = mapper.readValue(body, JsonRpcResponse.class);
        assertEquals(JsonRpcErrorCode.INTERNAL_ERROR, resp.error().code());
        assertNull(resp.error().data());
    }

    @Test
    void unknownPropertiesAreToleratedByCallers() throws Exception {
        // The SDK mapper disables FAIL_ON_UNKNOWN_PROPERTIES; a plain mapper configured the same way
        // must accept newer/extra fields without throwing.
        ObjectMapper lenient = new ObjectMapper()
                .disable(com.fasterxml.jackson.databind.DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES);
        String body = "{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"result\":1,\"futureField\":true}";
        JsonRpcResponse resp = lenient.readValue(body, JsonRpcResponse.class);
        assertEquals(1, resp.result().asInt());
    }
}
