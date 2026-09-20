(function (root, factory) {
  "use strict";

  var api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.AgnosticAIVideo = api;

  if (root.document) {
    api.init(root.document);
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function embedURL(videoId) {
    if (!/^[A-Za-z0-9_-]{11}$/.test(String(videoId || ""))) {
      return null;
    }
    return "https://www.youtube-nocookie.com/embed/" + videoId + "?autoplay=1&rel=0";
  }

  // The poster is a plain link to YouTube until it is clicked, so the page
  // ships no player and still works without JavaScript.
  function initVideoFacade(document) {
    var facade = document.querySelector("[data-video-facade]");
    var link = facade && facade.querySelector("[data-video-play]");
    if (!link) {
      return false;
    }

    link.addEventListener("click", function (event) {
      var src = embedURL(facade.dataset.videoId);
      if (!src) {
        return;
      }
      event.preventDefault();
      var iframe = document.createElement("iframe");
      iframe.src = src;
      iframe.title = facade.dataset.videoTitle || "";
      iframe.setAttribute("allow", "accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share");
      iframe.setAttribute("allowfullscreen", "");
      iframe.setAttribute("referrerpolicy", "strict-origin-when-cross-origin");
      link.replaceWith(iframe);
      iframe.focus();
    });
    return true;
  }

  function init(document) {
    initVideoFacade(document);
  }

  return {
    embedURL: embedURL,
    init: init,
    initVideoFacade: initVideoFacade
  };
});
