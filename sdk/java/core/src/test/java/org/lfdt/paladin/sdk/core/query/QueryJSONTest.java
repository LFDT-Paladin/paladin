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
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;

class QueryJSONTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void roundTripsFullQuery() throws Exception {
        QueryJSON query = QueryJSON.builder()
                .limit(25)
                .sort("created DESC")
                .equal("type", "private", QueryModifier.CASE_INSENSITIVE)
                .greaterThan("blockNumber", 100)
                .in("status", List.of("pending", "confirmed"))
                .isNull("failureMessage")
                .or(QueryJSON.builder().equal("domain", "noto"))
                .build();

        QueryJSON parsed = MAPPER.readValue(MAPPER.writeValueAsString(query), QueryJSON.class);
        assertEquals(query, parsed);
        assertEquals(25, parsed.limit());
        assertEquals("type", parsed.eq().get(0).field());
        assertTrue(parsed.eq().get(0).caseInsensitive());
        assertEquals(2, parsed.in().get(0).values().size());
        assertEquals("failureMessage", parsed.isNull().get(0).field());
        assertEquals(1, parsed.or().size());
        assertEquals("noto", parsed.or().get(0).eq().get(0).value().asText());
    }

    @Test
    void parsesNodeStyleJson() throws Exception {
        String json = "{"
                + "\"limit\":5,"
                + "\"sort\":[\"created\"],"
                + "\"gte\":[{\"field\":\"value\",\"value\":\"0x0a\"}],"
                + "\"null\":[{\"field\":\"contractAddress\",\"not\":true}]}";
        QueryJSON query = MAPPER.readValue(json, QueryJSON.class);
        assertEquals(5, query.limit());
        assertEquals(List.of("created"), query.sort());
        assertEquals("0x0a", query.gte().get(0).value().asText());
        assertTrue(query.isNull().get(0).not());
    }

    @Test
    void emptyQueryHasNonNullEmptyLists() throws Exception {
        QueryJSON query = MAPPER.readValue("{}", QueryJSON.class);
        assertTrue(query.eq().isEmpty());
        assertTrue(query.or().isEmpty());
        assertTrue(query.isNull().isEmpty());
        assertFalse(query.equals(QueryJSON.builder().limit(1).build()));
    }

    @Test
    void preservesNumericValuePrecision() throws Exception {
        // A large integer value must not be coerced through a double.
        QueryJSON query = QueryJSON.builder().equal("nonce", 9007199254740993L).build();
        QueryJSON parsed = MAPPER.readValue(MAPPER.writeValueAsString(query), QueryJSON.class);
        assertEquals(9007199254740993L, parsed.eq().get(0).value().asLong());
        assertEquals(query, parsed);
    }
}
