(function (root, factory) {
  "use strict";
  var api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root.document) api.init(root.document, root);
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function initFlow(flow, document, view) {
    if (flow.hasAttribute("data-hook-ready")) return null;
    var stages = Array.from(flow.querySelectorAll("[data-hook-stage]"));
    var example = Array.from(flow.querySelectorAll("[data-hook-phase]"));
    var controls = flow.querySelector("[data-hook-controls]");
    var playButton = flow.querySelector("[data-hook-play]");
    var nextButton = flow.querySelector("[data-hook-next]");
    var resetButton = flow.querySelector("[data-hook-reset]");
    var status = flow.querySelector("[data-hook-status]");
    var reducedNote = flow.querySelector("[data-hook-reduced]");
    if (!stages.length || !example.length || !controls || !playButton || !nextButton || !resetButton || !status || !reducedNote) return null;
    var motion = view.matchMedia ? view.matchMedia("(prefers-reduced-motion: reduce)") : { matches: false };
    var initialStatus = status.textContent;
    var index = -1;
    var playing = false;
    var timer;

    function cancelTimer() {
      if (timer !== undefined) view.clearTimeout(timer);
      timer = undefined;
    }

    function render() {
      var step = example[index];
      stages.forEach(function (stage) {
        var active = !!step && stage.getAttribute("data-hook-stage") === step.getAttribute("data-hook-phase");
        stage.classList.toggle("is-active", active);
        if (active) stage.setAttribute("aria-current", "step");
        else stage.removeAttribute("aria-current");
      });
      flow.setAttribute("data-hook-edit", step && step.hasAttribute("data-hook-edit") ? "true" : "false");
      status.textContent = step ? step.textContent : initialStatus;
      playButton.textContent = playing ? "Pause example" : index === example.length - 1 ? "Replay example" : "Play example";
      playButton.setAttribute("aria-pressed", playing ? "true" : "false");
      playButton.hidden = motion.matches;
      nextButton.disabled = index === example.length - 1;
      resetButton.disabled = index === -1;
      reducedNote.hidden = !motion.matches;
    }

    function pause() {
      cancelTimer();
      playing = false;
      render();
    }

    function tick() {
      timer = undefined;
      index += 1;
      if (index === example.length - 1) playing = false;
      render();
      if (playing) timer = view.setTimeout(tick, 1800);
    }

    function play() {
      if (playing) return pause();
      if (motion.matches || document.hidden) return;
      if (index === example.length - 1) index = -1;
      playing = true;
      if (index === -1) tick();
      else {
        render();
        timer = view.setTimeout(tick, 1800);
      }
    }

    function next() {
      cancelTimer();
      playing = false;
      if (index < example.length - 1) index += 1;
      render();
    }

    function reset() {
      cancelTimer();
      playing = false;
      index = -1;
      render();
    }

    function visibilityChanged() {
      if (document.hidden) pause();
    }

    function motionChanged() {
      if (motion.matches) pause();
      else render();
    }

    playButton.addEventListener("click", play);
    nextButton.addEventListener("click", next);
    resetButton.addEventListener("click", reset);
    document.addEventListener("visibilitychange", visibilityChanged);
    view.addEventListener("pagehide", pause);
    if (motion.addEventListener) motion.addEventListener("change", motionChanged);
    flow.setAttribute("data-hook-ready", "");
    controls.hidden = false;
    render();
    return {
      destroy: function () {
        pause();
        document.removeEventListener("visibilitychange", visibilityChanged);
        view.removeEventListener("pagehide", pause);
        if (motion.removeEventListener) motion.removeEventListener("change", motionChanged);
        playButton.removeEventListener("click", play);
        nextButton.removeEventListener("click", next);
        resetButton.removeEventListener("click", reset);
      }
    };
  }

  function init(document, view) {
    return Array.from(document.querySelectorAll("[data-hook-flow]")).map(function (flow) {
      return initFlow(flow, document, view);
    });
  }

  return { init: init, initFlow: initFlow };
});
