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
package org.lfdt.paladin.sdk.core.types;

import java.math.BigInteger;
import java.util.HexFormat;

/** Internal hex parsing/formatting helpers shared by the value types in this package. */
final class Hex {

  /** Lower-case, no-separator hex codec, matching the Go SDK's JSON output. */
  static final HexFormat FORMAT = HexFormat.of();

  private Hex() {}

  /**
   * Strips a leading {@code 0x} / {@code 0X} prefix if present, otherwise returns the input
   * unchanged.
   */
  static String strip0x(String s) {
    if (s.length() >= 2 && s.charAt(0) == '0' && (s.charAt(1) == 'x' || s.charAt(1) == 'X')) {
      return s.substring(2);
    }
    return s;
  }

  /**
   * Decodes a hex string (with or without a {@code 0x} prefix, any case) into bytes. An empty
   * string (or a bare {@code "0x"}) decodes to an empty array.
   */
  static byte[] decode(String s) {
    if (s == null) {
      throw new IllegalArgumentException("hex string must not be null");
    }
    String body = strip0x(s);
    if (body.isEmpty()) {
      return new byte[0];
    }
    try {
      return FORMAT.parseHex(body);
    } catch (IllegalArgumentException e) {
      throw new IllegalArgumentException("invalid hex string: \"" + s + "\"", e);
    }
  }

  /**
   * Parses a base-aware integer string the way the Go SDK does for JSON numbers: a {@code
   * 0x}/{@code 0X} prefix (optionally signed) is read as hex, anything else as decimal.
   */
  static BigInteger parseBigInteger(String s) {
    if (s == null) {
      throw new IllegalArgumentException("integer string must not be null");
    }
    String t = s.trim();
    if (t.isEmpty()) {
      throw new IllegalArgumentException("integer string must not be empty");
    }
    boolean negative = false;
    if (t.charAt(0) == '+') {
      t = t.substring(1);
    } else if (t.charAt(0) == '-') {
      negative = true;
      t = t.substring(1);
    }
    try {
      BigInteger value =
          (t.length() >= 2 && t.charAt(0) == '0' && (t.charAt(1) == 'x' || t.charAt(1) == 'X'))
              ? new BigInteger(t.substring(2), 16)
              : new BigInteger(t, 10);
      return negative ? value.negate() : value;
    } catch (NumberFormatException e) {
      throw new IllegalArgumentException("invalid integer string: \"" + s + "\"", e);
    }
  }
}
