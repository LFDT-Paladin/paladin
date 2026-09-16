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
package org.lfdt.paladin.sdk.core.privacygroup;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.transaction.PublicTxOptions;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class PrivacyGroupTypesTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  private static final String GROUP_ID = "0xfeed";
  private static final String ADDRESS = "0x" + "11".repeat(20);
  private static final String SALT = "0x" + "22".repeat(32);
  private static final String SCHEMA = "0x" + "33".repeat(32);

  private static void assertValueEquality(
      final Object first, final Object equal, final Object unequal) {
    assertEquals(first, first);
    assertEquals(first, equal);
    assertEquals(first.hashCode(), equal.hashCode());
    assertNotEquals(first, unequal);
    assertNotEquals(first, null);
    assertNotEquals(first, "different type");
  }

  // ---- PrivacyGroup -------------------------------------------------------

  @Test
  void privacyGroupRoundTripsFully() throws Exception {
    final UUID genesisTx = UUID.randomUUID();
    final String json =
        "{\"id\":\""
            + GROUP_ID
            + "\",\"domain\":\"pente\",\"created\":\"1700000000000000000\",\"name\":\"g1\","
            + "\"members\":[\"me@node1\",\"you@node2\"],"
            + "\"properties\":{\"k\":\"v\"},\"configuration\":{\"evmVersion\":\"shanghai\"},"
            + "\"genesisSalt\":\""
            + SALT
            + "\",\"genesisSchema\":\""
            + SCHEMA
            + "\",\"genesisTransaction\":\""
            + genesisTx
            + "\",\"contractAddress\":\""
            + ADDRESS
            + "\"}";

    final PrivacyGroup group = MAPPER.readValue(json, PrivacyGroup.class);

    assertEquals(HexBytes.fromString(GROUP_ID), group.id());
    assertEquals("pente", group.domain());
    assertEquals(Timestamp.ofUnixNano(1700000000000000000L), group.created());
    assertEquals("g1", group.name());
    assertEquals(List.of("me@node1", "you@node2"), group.members());
    assertEquals(Map.of("k", "v"), group.properties());
    assertEquals(Map.of("evmVersion", "shanghai"), group.configuration());
    assertEquals(Bytes32.fromString(SALT), group.genesisSalt());
    assertEquals(Bytes32.fromString(SCHEMA), group.genesisSchema());
    assertEquals(genesisTx, group.genesisTransaction());
    assertEquals(EthAddress.fromString(ADDRESS), group.contractAddress());
    assertTrue(group.toString().contains("domain=pente"));

    final PrivacyGroup reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(group), PrivacyGroup.class);
    assertEquals(group.id(), reparsed.id());
    assertEquals(group.members(), reparsed.members());
    assertEquals(group.contractAddress(), reparsed.contractAddress());
  }

  @Test
  void privacyGroupNormalizesUnsetCollectionsAndTimestamp() throws Exception {
    final PrivacyGroup group =
        MAPPER.readValue("{\"domain\":\"pente\",\"created\":\"0\"}", PrivacyGroup.class);

    assertNull(group.created());
    assertNull(group.id());
    assertNull(group.contractAddress());
    assertTrue(group.members().isEmpty());
    assertTrue(group.properties().isEmpty());
    assertTrue(group.configuration().isEmpty());

    // Null-valued fields drop out; the never-null collections still serialize.
    final String json = MAPPER.writeValueAsString(group);
    assertFalse(json.contains("\"created\""));
    assertFalse(json.contains("\"contractAddress\""));
    assertTrue(json.contains("\"members\":[]"));
  }

  // ---- PrivacyGroupInput / PrivacyGroupTXOptions --------------------------

  @Test
  void privacyGroupInputBuilderRoundTrips() throws Exception {
    final PrivacyGroupInput input =
        PrivacyGroupInput.builder("pente")
            .name("g1")
            .member("me@node1")
            .members(List.of("you@node2"))
            .property("k", "v")
            .properties(Map.of("k2", "v2"))
            .configuration("evmVersion", "shanghai")
            .configuration(Map.of("endorsementType", "group_scoped_identities"))
            .transactionOptions(
                PrivacyGroupTXOptions.builder()
                    .idempotencyKey("idem-1")
                    .publicTxOptions(
                        PublicTxOptions.builder()
                            .gas(HexUint64.of(100_000L))
                            .value(HexUint256.of(7L))
                            .maxPriorityFeePerGas(HexUint256.of(1L))
                            .maxFeePerGas(HexUint256.of(2L))
                            .build())
                    .build())
            .build();

    assertEquals(List.of("me@node1", "you@node2"), input.members());
    assertEquals(Map.of("k", "v", "k2", "v2"), input.properties());
    assertEquals(2, input.configuration().size());
    assertTrue(input.toString().contains("domain=pente"));

    final PrivacyGroupInput parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(input), PrivacyGroupInput.class);
    assertEquals("pente", parsed.domain());
    assertEquals("g1", parsed.name());
    assertEquals(input.members(), parsed.members());
    assertEquals(input.properties(), parsed.properties());

    final PrivacyGroupTXOptions options = parsed.transactionOptions();
    assertEquals("idem-1", options.idempotencyKey());
    assertEquals(HexUint64.of(100_000L), options.publicTxOptions().gas());
    assertEquals(HexUint256.of(7L), options.publicTxOptions().value());
    assertEquals(HexUint256.of(1L), options.publicTxOptions().maxPriorityFeePerGas());
    assertEquals(HexUint256.of(2L), options.publicTxOptions().maxFeePerGas());
    assertTrue(options.toString().contains("idem-1"));
  }

  @Test
  void privacyGroupInputOmitsEmptyOptionalFields() throws Exception {
    final PrivacyGroupInput input = PrivacyGroupInput.builder("pente").member("me@node1").build();

    final String json = MAPPER.writeValueAsString(input);
    assertEquals("{\"domain\":\"pente\",\"members\":[\"me@node1\"]}", json);
  }

  @Test
  void privacyGroupTxOptionsOmitsEverythingWhenEmpty() throws Exception {
    assertEquals("{}", MAPPER.writeValueAsString(PrivacyGroupTXOptions.builder().build()));
  }

  @Test
  void emptyTransactionOptionsRetainValueEqualityAfterRoundTrip() throws Exception {
    final PrivacyGroupTXOptions options = PrivacyGroupTXOptions.builder().build();
    final PrivacyGroupTXOptions parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(options), PrivacyGroupTXOptions.class);
    final PrivacyGroupTXOptions explicitlyEmpty =
        PrivacyGroupTXOptions.builder().publicTxOptions(PublicTxOptions.builder().build()).build();

    assertEquals(options, parsed);
    assertEquals(options, explicitlyEmpty);
    assertEquals(options.hashCode(), parsed.hashCode());
    assertEquals(options.hashCode(), explicitlyEmpty.hashCode());
    assertEquals(PublicTxOptions.builder().build(), options.publicTxOptions());
  }

  // ---- PrivacyGroupEVMTXInput ---------------------------------------------

  @Test
  void evmTxInputFlattensTxFieldsAndNestsPublicTxOptions() throws Exception {
    final PrivacyGroupEVMTXInput tx =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .idempotencyKey("idem-2")
            .from("me@node1")
            .to(EthAddress.fromString(ADDRESS))
            .gas(HexUint64.of(500_000L))
            .value(HexUint256.of(3L))
            .input(MAPPER.readTree("{\"amount\":\"10\"}"))
            .function(AbiEntry.function("transfer").build())
            .bytecode(HexBytes.fromString("0xcafe"))
            .publicTxOptions(PublicTxOptions.builder().gas(HexUint64.of(21_000L)).build())
            .build();

    final var node = MAPPER.valueToTree(tx);
    // The EVM transaction fields sit at the top level...
    assertEquals("me@node1", node.get("from").asText());
    assertEquals("0x7a120", node.get("gas").asText());
    assertEquals("10", node.get("input").get("amount").asText());
    assertEquals("transfer", node.get("function").get("name").asText());
    // ...while the base-ledger options stay nested under their own key.
    assertEquals("0x5208", node.get("publicTxOptions").get("gas").asText());

    final PrivacyGroupEVMTXInput parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(tx), PrivacyGroupEVMTXInput.class);
    assertEquals("idem-2", parsed.idempotencyKey());
    assertEquals("pente", parsed.domain());
    assertEquals(HexBytes.fromString(GROUP_ID), parsed.group());
    assertEquals(EthAddress.fromString(ADDRESS), parsed.to());
    assertEquals(HexUint256.of(3L), parsed.value());
    assertEquals(HexBytes.fromString("0xcafe"), parsed.bytecode());
    assertEquals(HexUint64.of(21_000L), parsed.publicTxOptions().gas());
    assertTrue(parsed.toString().contains("domain=pente"));
    assertTrue(parsed.publicTxOptions().toString().contains("gas="));
  }

  @Test
  void evmTxInputOmitsUnsetFields() throws Exception {
    final PrivacyGroupEVMTXInput tx =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID)).build();

    assertEquals(
        "{\"domain\":\"pente\",\"group\":\"" + GROUP_ID + "\"}", MAPPER.writeValueAsString(tx));
  }

  // ---- PrivacyGroupEVMCall ------------------------------------------------

  @Test
  void evmCallFlattensCallOptions() throws Exception {
    final PrivacyGroupEVMTXInput input =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .from("me@node1")
            .to(EthAddress.fromString(ADDRESS))
            .gas(HexUint64.of(50_000L))
            .value(HexUint256.of(0L))
            .input(MAPPER.readTree("[]"))
            .function(AbiEntry.function("balanceOf").build())
            .bytecode(HexBytes.fromString("0xbeef"))
            .build();
    final PrivacyGroupEVMCall call =
        PrivacyGroupEVMCall.builder(input)
            .block("latest")
            .dataFormat("mode=object,number=string")
            .build();

    final var node = MAPPER.valueToTree(call);
    assertEquals("pente", node.get("domain").asText());
    assertEquals(GROUP_ID, node.get("group").asText());
    assertEquals("me@node1", node.get("from").asText());
    assertEquals(ADDRESS, node.get("to").asText());
    assertEquals("0xc350", node.get("gas").asText());
    assertEquals("0x00", node.get("value").asText());
    assertTrue(node.get("input").isArray());
    assertEquals("balanceOf", node.get("function").get("name").asText());
    assertEquals("0xbeef", node.get("bytecode").asText());
    assertEquals("latest", node.get("block").asText());
    assertEquals("mode=object,number=string", node.get("dataFormat").asText());
    assertEquals("latest", call.block());
    assertEquals("mode=object,number=string", call.dataFormat());
    assertEquals(input, call.input());
    assertTrue(call.toString().contains("domain=pente"));
    final PrivacyGroupEVMCall parsed = MAPPER.treeToValue(node, PrivacyGroupEVMCall.class);
    assertEquals(node, MAPPER.valueToTree(parsed));
    assertEquals("latest", parsed.block());
    assertEquals("mode=object,number=string", parsed.dataFormat());
  }

  @Test
  void evmCallOmitsUnsetFields() throws Exception {
    final PrivacyGroupEVMTXInput input =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID)).build();
    final PrivacyGroupEVMCall call = PrivacyGroupEVMCall.builder(input).build();

    assertEquals(
        "{\"domain\":\"pente\",\"group\":\"" + GROUP_ID + "\"}", MAPPER.writeValueAsString(call));
    assertNull(call.block());
    assertNull(call.dataFormat());
    final PrivacyGroupEVMCall parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(call), PrivacyGroupEVMCall.class);
    assertEquals(MAPPER.valueToTree(call), MAPPER.valueToTree(parsed));
  }

  @Test
  void evmCallRejectsNullInput() {
    assertThrows(NullPointerException.class, () -> PrivacyGroupEVMCall.builder(null).build());
  }

  @Test
  void evmCallDoesNotSendTransactionSubmissionOptions() {
    final PrivacyGroupEVMTXInput input =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .idempotencyKey("send-only")
            .gas(HexUint64.of(50_000L))
            .publicTxOptions(PublicTxOptions.builder().gas(HexUint64.of(21_000L)).build())
            .build();
    final var node = MAPPER.valueToTree(PrivacyGroupEVMCall.builder(input).build());
    assertFalse(node.has("idempotencyKey"));
    assertFalse(node.has("publicTxOptions"));
    assertEquals("0xc350", node.get("gas").asText());
    assertEquals("pente", node.get("domain").asText());
  }

  // ---- messages -----------------------------------------------------------

  @Test
  void messageInputRoundTrips() throws Exception {
    final UUID correlationId = UUID.randomUUID();
    final PrivacyGroupMessageInput message =
        PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .correlationId(correlationId)
            .topic("orders")
            .data(MAPPER.readTree("{\"hello\":\"world\"}"))
            .build();

    final PrivacyGroupMessageInput parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(message), PrivacyGroupMessageInput.class);

    assertEquals(correlationId, parsed.correlationId());
    assertEquals("pente", parsed.domain());
    assertEquals(HexBytes.fromString(GROUP_ID), parsed.group());
    assertEquals("orders", parsed.topic());
    assertEquals("world", parsed.data().get("hello").asText());
    assertTrue(parsed.toString().contains("topic=orders"));
  }

  @Test
  void messageInputOmitsUnsetFields() throws Exception {
    final PrivacyGroupMessageInput message =
        PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID)).build();

    assertEquals(
        "{\"domain\":\"pente\",\"group\":\"" + GROUP_ID + "\"}",
        MAPPER.writeValueAsString(message));
  }

  @Test
  void messageFlattensInputFieldsAlongsideDeliveryMetadata() throws Exception {
    final UUID id = UUID.randomUUID();
    final String json =
        "{\"id\":\""
            + id
            + "\",\"localSequence\":42,\"sent\":\"1700000000000000000\","
            + "\"received\":\"1700000000000000001\",\"node\":\"node2\","
            + "\"domain\":\"pente\",\"group\":\""
            + GROUP_ID
            + "\",\"topic\":\"orders\",\"data\":{\"hello\":\"world\"}}";

    final PrivacyGroupMessage message = MAPPER.readValue(json, PrivacyGroupMessage.class);

    assertEquals(id, message.id());
    assertEquals(42L, message.localSequence());
    assertEquals(Timestamp.ofUnixNano(1700000000000000000L), message.sent());
    assertEquals(Timestamp.ofUnixNano(1700000000000000001L), message.received());
    assertEquals("node2", message.node());
    assertNull(message.correlationId());
    assertEquals("pente", message.domain());
    assertEquals(HexBytes.fromString(GROUP_ID), message.group());
    assertEquals("orders", message.topic());
    assertEquals("world", message.data().get("hello").asText());
    assertTrue(message.toString().contains("localSequence=42"));

    final PrivacyGroupMessage reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(message), PrivacyGroupMessage.class);
    assertEquals(message.id(), reparsed.id());
    assertEquals(message.sent(), reparsed.sent());
  }

  @Test
  void messageNormalizesZeroTimestamps() throws Exception {
    final PrivacyGroupMessage message =
        MAPPER.readValue("{\"sent\":\"0\",\"received\":\"0\"}", PrivacyGroupMessage.class);

    assertNull(message.sent());
    assertNull(message.received());
    assertNull(message.data());
    assertEquals(0L, message.localSequence());
  }

  // ---- listeners ----------------------------------------------------------

  @Test
  void messageListenerRoundTrips() throws Exception {
    final PrivacyGroupMessageListener listener =
        PrivacyGroupMessageListener.builder("orders-listener")
            .started(true)
            .filters(
                PrivacyGroupMessageListenerFilters.builder()
                    .sequenceAbove(10L)
                    .domain("pente")
                    .group(HexBytes.fromString(GROUP_ID))
                    .topic("orders")
                    .build())
            .options(PrivacyGroupMessageListenerOptions.builder().excludeLocal(true).build())
            .build();

    // The builder never sets `created` — that is server-assigned.
    assertNull(listener.created());

    final PrivacyGroupMessageListener parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(listener), PrivacyGroupMessageListener.class);

    assertEquals("orders-listener", parsed.name());
    assertTrue(parsed.started());
    assertEquals(10L, parsed.filters().sequenceAbove());
    assertEquals("pente", parsed.filters().domain());
    assertEquals(HexBytes.fromString(GROUP_ID), parsed.filters().group());
    assertEquals("orders", parsed.filters().topic());
    assertTrue(parsed.options().excludeLocal());
    assertTrue(parsed.toString().contains("orders-listener"));
    assertTrue(parsed.filters().toString().contains("topic=orders"));
    assertTrue(parsed.options().toString().contains("excludeLocal=true"));
  }

  @Test
  void messageListenerNormalizesZeroCreatedAndOmitsUnset() throws Exception {
    final PrivacyGroupMessageListener listener =
        MAPPER.readValue("{\"name\":\"l1\",\"created\":\"0\"}", PrivacyGroupMessageListener.class);

    assertNull(listener.created());
    assertNull(listener.started());
    assertNull(listener.filters());
    assertNull(listener.options());
    assertEquals("{\"name\":\"l1\"}", MAPPER.writeValueAsString(listener));
  }

  @Test
  void listenerFiltersOmitEverythingWhenUnset() throws Exception {
    assertEquals(
        "{}", MAPPER.writeValueAsString(PrivacyGroupMessageListenerFilters.builder().build()));
  }

  // ---- PublicTxOptions ----------------------------------------------------

  @Test
  void publicTxOptionsRoundTripsAndComparesByValue() throws Exception {
    final PublicTxOptions options =
        PublicTxOptions.builder()
            .gas(HexUint64.of(21_000L))
            .value(HexUint256.of(5L))
            .maxPriorityFeePerGas(HexUint256.of(1L))
            .maxFeePerGas(HexUint256.of(2L))
            .build();

    final PublicTxOptions parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(options), PublicTxOptions.class);

    assertEquals(options, parsed);
    assertEquals(options.hashCode(), parsed.hashCode());
    assertEquals(options, options);
    assertFalse(options.equals(PublicTxOptions.builder().build()));
    assertFalse(options.equals("not-options"));
    assertEquals(HexUint64.of(21_000L), parsed.gas());
    assertEquals(HexUint256.of(5L), parsed.value());
    assertEquals(HexUint256.of(1L), parsed.maxPriorityFeePerGas());
    assertEquals(HexUint256.of(2L), parsed.maxFeePerGas());
  }

  @Test
  void publicTxOptionsOmitsEverythingWhenEmpty() throws Exception {
    assertEquals("{}", MAPPER.writeValueAsString(PublicTxOptions.builder().build()));
  }

  @Test
  void privacyGroupInputDefaultsAbsentCollectionsToEmpty() throws Exception {
    final PrivacyGroupInput input =
        MAPPER.readValue("{\"domain\":\"pente\"}", PrivacyGroupInput.class);

    assertEquals("pente", input.domain());
    assertTrue(input.members().isEmpty());
    assertTrue(input.properties().isEmpty());
    assertTrue(input.configuration().isEmpty());
    assertNull(input.name());
    assertNull(input.transactionOptions());
    assertEquals("{\"domain\":\"pente\",\"members\":[]}", MAPPER.writeValueAsString(input));
  }

  @Test
  void messageListenerKeepsNonZeroCreatedTimestamp() throws Exception {
    final PrivacyGroupMessageListener listener =
        MAPPER.readValue(
            "{\"name\":\"l1\",\"created\":\"2024-01-01T00:00:00Z\",\"started\":true}",
            PrivacyGroupMessageListener.class);

    assertEquals(Timestamp.fromString("2024-01-01T00:00:00Z"), listener.created());
    assertTrue(listener.started());
    final PrivacyGroupMessageListener reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(listener), PrivacyGroupMessageListener.class);
    assertEquals(listener.created(), reparsed.created());
  }

  @Test
  void privacyGroupValueObjectsImplementEquality() throws Exception {
    final String groupJson = "{\"id\":\"" + GROUP_ID + "\",\"domain\":\"pente\",\"name\":\"g1\"}";
    final PrivacyGroup group = MAPPER.readValue(groupJson, PrivacyGroup.class);
    final PrivacyGroup equalGroup = MAPPER.readValue(groupJson, PrivacyGroup.class);
    final PrivacyGroup otherGroup =
        MAPPER.readValue(groupJson.replace("\"g1\"", "\"g2\""), PrivacyGroup.class);
    assertValueEquality(group, equalGroup, otherGroup);

    final PrivacyGroupTXOptions txOptions =
        PrivacyGroupTXOptions.builder()
            .idempotencyKey("idem")
            .publicTxOptions(PublicTxOptions.builder().gas(HexUint64.of(21_000L)).build())
            .build();
    final PrivacyGroupTXOptions equalTxOptions =
        PrivacyGroupTXOptions.builder()
            .idempotencyKey("idem")
            .publicTxOptions(PublicTxOptions.builder().gas(HexUint64.of(21_000L)).build())
            .build();
    assertValueEquality(
        txOptions, equalTxOptions, PrivacyGroupTXOptions.builder().idempotencyKey("other").build());

    final PrivacyGroupInput groupInput =
        PrivacyGroupInput.builder("pente")
            .name("g1")
            .member("me@node1")
            .transactionOptions(txOptions)
            .build();
    final PrivacyGroupInput equalGroupInput =
        PrivacyGroupInput.builder("pente")
            .name("g1")
            .member("me@node1")
            .transactionOptions(equalTxOptions)
            .build();
    assertValueEquality(groupInput, equalGroupInput, PrivacyGroupInput.builder("other").build());

    final PrivacyGroupEVMTXInput evmInput =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .from("me@node1")
            .gas(HexUint64.of(50_000L))
            .build();
    final PrivacyGroupEVMTXInput equalEvmInput =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .from("me@node1")
            .gas(HexUint64.of(50_000L))
            .build();
    assertValueEquality(
        evmInput,
        equalEvmInput,
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString("0xbeef")).build());

    final PrivacyGroupEVMCall evmCall =
        PrivacyGroupEVMCall.builder(evmInput).block("latest").build();
    final PrivacyGroupEVMCall equalEvmCall =
        PrivacyGroupEVMCall.builder(equalEvmInput).block("latest").build();
    assertValueEquality(
        evmCall, equalEvmCall, PrivacyGroupEVMCall.builder(evmInput).block("pending").build());
  }

  @Test
  void privacyGroupMessageValueObjectsImplementEquality() throws Exception {
    final PrivacyGroupMessageInput messageInput =
        PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .topic("orders")
            .data(MAPPER.readTree("{\"value\":1}"))
            .build();
    final PrivacyGroupMessageInput equalMessageInput =
        PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .topic("orders")
            .data(MAPPER.readTree("{\"value\":1}"))
            .build();
    assertValueEquality(
        messageInput,
        equalMessageInput,
        PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .topic("other")
            .build());

    final String messageJson =
        "{\"id\":\"00000000-0000-0000-0000-000000000001\",\"localSequence\":1,"
            + "\"domain\":\"pente\",\"group\":\""
            + GROUP_ID
            + "\",\"topic\":\"orders\",\"data\":{\"value\":1}}";
    final PrivacyGroupMessage message = MAPPER.readValue(messageJson, PrivacyGroupMessage.class);
    final PrivacyGroupMessage equalMessage =
        MAPPER.readValue(messageJson, PrivacyGroupMessage.class);
    final PrivacyGroupMessage otherMessage =
        MAPPER.readValue(
            messageJson.replace("\"localSequence\":1", "\"localSequence\":2"),
            PrivacyGroupMessage.class);
    assertValueEquality(message, equalMessage, otherMessage);

    final PrivacyGroupMessageListenerFilters filters =
        PrivacyGroupMessageListenerFilters.builder().domain("pente").topic("orders").build();
    final PrivacyGroupMessageListenerFilters equalFilters =
        PrivacyGroupMessageListenerFilters.builder().domain("pente").topic("orders").build();
    assertValueEquality(
        filters,
        equalFilters,
        PrivacyGroupMessageListenerFilters.builder().domain("pente").topic("other").build());

    final PrivacyGroupMessageListenerOptions options =
        PrivacyGroupMessageListenerOptions.builder().excludeLocal(true).build();
    final PrivacyGroupMessageListenerOptions equalOptions =
        PrivacyGroupMessageListenerOptions.builder().excludeLocal(true).build();
    assertValueEquality(
        options,
        equalOptions,
        PrivacyGroupMessageListenerOptions.builder().excludeLocal(false).build());

    final PrivacyGroupMessageListener listener =
        PrivacyGroupMessageListener.builder("listener")
            .started(true)
            .filters(filters)
            .options(options)
            .build();
    final PrivacyGroupMessageListener equalListener =
        PrivacyGroupMessageListener.builder("listener")
            .started(true)
            .filters(equalFilters)
            .options(equalOptions)
            .build();
    assertValueEquality(
        listener, equalListener, PrivacyGroupMessageListener.builder("other-listener").build());
  }
}
