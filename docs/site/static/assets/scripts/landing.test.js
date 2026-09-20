"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { detectOS, embedURL, nextTabIndex } = require("./landing.js");

test("detects supported desktop operating systems", function () {
  assert.equal(detectOS("macOS", "", 0), "macos");
  assert.equal(detectOS("Win32", "", 0), "windows");
  assert.equal(detectOS("Linux x86_64", "", 0), "linux");
});

test("uses the user agent when platform data is unavailable", function () {
  assert.equal(detectOS("", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", 0), "windows");
  assert.equal(detectOS("", "Mozilla/5.0 (X11; Linux x86_64)", 0), "linux");
});

test("does not recommend desktop installers on mobile or ChromeOS", function () {
  assert.equal(detectOS("MacIntel", "Mozilla/5.0 (iPad)", 5), "other");
  assert.equal(detectOS("Linux armv8l", "Mozilla/5.0 (Linux; Android 15)", 5), "other");
  assert.equal(detectOS("Linux x86_64", "Mozilla/5.0 (X11; CrOS x86_64)", 0), "other");
});

test("leaves unknown platforms on the neutral fallback", function () {
  assert.equal(detectOS("", "", 0), "other");
  assert.equal(detectOS("FreeBSD amd64", "Mozilla/5.0", 0), "other");
});

test("builds a privacy-enhanced autoplay embed URL for valid video ids", function () {
  assert.equal(embedURL("uEG6ITlqyHU"), "https://www.youtube-nocookie.com/embed/uEG6ITlqyHU?autoplay=1&rel=0");
});

test("rejects video ids that are not plain YouTube ids", function () {
  assert.equal(embedURL("../evil"), null);
  assert.equal(embedURL(""), null);
  assert.equal(embedURL("uEG6ITlqyHU/x"), null);
});

test("moves the output tablist selection with the arrow keys, wrapping at both ends", function () {
  assert.equal(nextTabIndex("ArrowDown", 0, 5), 1);
  assert.equal(nextTabIndex("ArrowRight", 4, 5), 0);
  assert.equal(nextTabIndex("ArrowUp", 0, 5), 4);
  assert.equal(nextTabIndex("ArrowLeft", 3, 5), 2);
});

test("jumps the output tablist to the first and last file", function () {
  assert.equal(nextTabIndex("Home", 3, 5), 0);
  assert.equal(nextTabIndex("End", 1, 5), 4);
});

test("leaves keys the output tablist does not own to the browser", function () {
  assert.equal(nextTabIndex("Tab", 2, 5), -1);
  assert.equal(nextTabIndex("a", 2, 5), -1);
  assert.equal(nextTabIndex("ArrowDown", 0, 0), -1);
});
