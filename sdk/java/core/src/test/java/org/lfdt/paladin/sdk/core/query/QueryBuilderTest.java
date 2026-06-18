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

package org.lfdt.paladin.sdk.core.query;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class QueryBuilderTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    /** Parses a JSON string into a tree so comparisons ignore key ordering. */
    private static com.fasterxml.jackson.databind.JsonNode tree(String json) throws Exception {
        return MAPPER.readTree(json);
    }

    @Test
    void buildsFullQueryMatchingGoReference() throws Exception {
        // Mirrors sdk/go/pkg/query TestQuery.
        String expected = "{"
                + "\"limit\":10,"
                + "\"sort\":[\"field1 DESC\",\"field2\"],"
                + "\"eq\":[{\"field\":\"field1\",\"value\":\"value1\"},"
                + "{\"field\":\"field12\",\"value\":\"value12\",\"not\":true,\"caseInsensitive\":true}],"
                + "\"neq\":[{\"field\":\"field2\",\"value\":\"value2\"}],"
                + "\"like\":[{\"field\":\"field3\",\"value\":\"some value\"}],"
                + "\"lt\":[{\"field\":\"field4\",\"value\":12345}],"
                + "\"lte\":[{\"field\":\"field5\",\"value\":23456}],"
                + "\"gt\":[{\"field\":\"field6\",\"value\":34567}],"
                + "\"gte\":[{\"field\":\"field7\",\"value\":45678}],"
                + "\"in\":[{\"field\":\"field8\",\"values\":[\"a\",\"b\",\"c\"]}],"
                + "\"nin\":[{\"field\":\"field9\",\"values\":[\"x\",\"y\",\"z\"]}],"
                + "\"null\":[{\"field\":\"field10\",\"not\":true},{\"field\":\"field11\"}]}";

        QueryJSON query = QueryJSON.builder()
                .limit(10)
                .sort("field1 DESC").sort("field2")
                .equal("field1", "value1", QueryModifier.CASE_SENSITIVE)
                .notEqual("field2", "value2")
                .like("field3", "some value")
                .lessThan("field4", 12345)
                .lessThanOrEqual("field5", 23456)
                .greaterThan("field6", 34567)
                .greaterThanOrEqual("field7", 45678)
                .in("field8", List.of("a", "b", "c"))
                .notIn("field9", List.of("x", "y", "z"))
                .isNotNull("field10")
                .isNull("field11")
                .equal("field12", "value12", QueryModifier.NOT, QueryModifier.CASE_INSENSITIVE)
                .build();

        assertEquals(tree(expected), tree(MAPPER.writeValueAsString(query)));
        // toJson() (via the SDK-wide mapper) yields the same document.
        assertEquals(tree(expected), tree(query.toJson()));
    }

    @Test
    void buildsOrQuery() throws Exception {
        String expected = "{\"or\":["
                + "{\"eq\":[{\"field\":\"field1\",\"value\":\"value1\"}],"
                + "\"neq\":[{\"field\":\"field2\",\"value\":\"value2\"}]},"
                + "{\"eq\":[{\"field\":\"field3\",\"value\":\"value3\"}],"
                + "\"neq\":[{\"field\":\"field4\",\"value\":\"value4\"}]}]}";

        QueryJSON query = QueryJSON.builder()
                .or(QueryJSON.builder().equal("field1", "value1").notEqual("field2", "value2"))
                .or(QueryJSON.builder().equal("field3", "value3").notEqual("field4", "value4"))
                .build();

        assertEquals(tree(expected), tree(MAPPER.writeValueAsString(query)));
    }

    @Test
    void orBranchesIgnoreLimitAndSort() throws Exception {
        // A child builder's limit/sort must not leak into the OR branch.
        QueryJSON query = QueryJSON.builder()
                .or(QueryJSON.builder().limit(99).sort("x").equal("a", 1))
                .build();
        String json = MAPPER.writeValueAsString(query);
        assertEquals(tree("{\"or\":[{\"eq\":[{\"field\":\"a\",\"value\":1}]}]}"), tree(json));
    }

    @Test
    void emptyQuerySerializesToEmptyObject() throws Exception {
        assertEquals("{}", MAPPER.writeValueAsString(QueryJSON.builder().build()));
    }

    @Test
    void zeroLimitIsEmittedButNullLimitIsOmitted() throws Exception {
        assertTrue(MAPPER.writeValueAsString(QueryJSON.builder().limit(0).build()).contains("\"limit\":0"));
        assertEquals("{}", MAPPER.writeValueAsString(QueryJSON.builder().build()));
    }

    @Test
    void valueTypesSerializeThroughTheirOwnRepresentation() throws Exception {
        EthAddress addr = EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A");
        QueryJSON query = QueryJSON.builder().equal("to", addr).build();
        assertEquals(
                tree("{\"eq\":[{\"field\":\"to\",\"value\":\"0x05d936207f04d81a85881b72a0d17854ee8be45a\"}]}"),
                tree(MAPPER.writeValueAsString(query)));
    }

    @Test
    void appendsRatherThanReplacesRepeatedOperators() throws Exception {
        QueryJSON query = QueryJSON.builder().equal("a", 1).equal("b", 2).build();
        assertEquals(2, query.eq().size());
        assertEquals("a", query.eq().get(0).field());
        assertEquals("b", query.eq().get(1).field());
    }
}
