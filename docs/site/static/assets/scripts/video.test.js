"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { embedURL } = require("./video.js");

test("builds a privacy-enhanced autoplay embed URL for valid video ids", function () {
  assert.equal(embedURL("uEG6ITlqyHU"), "https://www.youtube-nocookie.com/embed/uEG6ITlqyHU?autoplay=1&rel=0");
});

test("rejects video ids that are not plain YouTube ids", function () {
  assert.equal(embedURL("../evil"), null);
  assert.equal(embedURL(""), null);
  assert.equal(embedURL("uEG6ITlqyHU/x"), null);
});
