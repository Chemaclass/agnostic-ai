(function () {
  "use strict";

  function copyText(value) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(value);
    }
    return new Promise(function (resolve, reject) {
      var field = document.createElement("textarea");
      field.value = value;
      field.setAttribute("readonly", "");
      field.style.position = "fixed";
      field.style.opacity = "0";
      document.body.appendChild(field);
      field.select();
      try {
        if (!document.execCommand("copy")) {
          throw new Error("copy command failed");
        }
        resolve();
      } catch (error) {
        reject(error);
      } finally {
        field.remove();
      }
    });
  }

  function enableScrollFocus(element, label) {
    if (element.hasAttribute("tabindex")) {
      return;
    }
    function update() {
      var overflows = element.scrollWidth > element.clientWidth;
      if (overflows) {
        element.setAttribute("tabindex", "0");
        element.setAttribute("aria-label", label);
        if (element.tagName === "PRE") {
          element.setAttribute("role", "region");
        }
      } else {
        element.removeAttribute("tabindex");
        element.removeAttribute("aria-label");
        if (element.tagName === "PRE") {
          element.removeAttribute("role");
        }
      }
    }
    update();
    if (window.ResizeObserver) {
      new ResizeObserver(update).observe(element);
    }
  }

  Array.prototype.forEach.call(document.querySelectorAll(".docs-content pre, .article-body pre"), function (pre) {
    var code = pre.querySelector("code");
    if (pre.closest("[data-copy-container]") || !code) {
      return;
    }
    var language = pre.getAttribute("data-lang") || code.getAttribute("data-lang") ||
      (code.className.match(/\blanguage-([\w-]+)/) || [])[1] || "code";
    var block = document.createElement("div");
    block.className = "code-block";
    block.setAttribute("data-copy-container", "");
    pre.parentNode.insertBefore(block, pre);
    var toolbar = document.createElement("div");
    toolbar.className = "code-toolbar";
    var label = document.createElement("span");
    label.textContent = /^(bash|sh|shell|zsh)$/.test(language) ? "Shell" : language.toUpperCase();
    toolbar.appendChild(label);
    block.appendChild(toolbar);
    block.appendChild(pre);
    var button = document.createElement("button");
    button.type = "button";
    button.className = "code-copy";
    button.setAttribute("data-copy", "");
    button.setAttribute("aria-label", "Copy " + label.textContent + " code");
    button.setAttribute("aria-live", "polite");
    button.textContent = "Copy";
    toolbar.appendChild(button);
    enableScrollFocus(pre, label.textContent + " example");
  });

  Array.prototype.forEach.call(document.querySelectorAll(".article-body table"), function (table) {
    var headers = Array.prototype.map.call(table.querySelectorAll("thead th"), function (cell) {
      return cell.textContent;
    });
    enableScrollFocus(table, headers.join(", ") || "Release details");
  });

  Array.prototype.forEach.call(document.querySelectorAll("[data-copy]"), function (button) {
    button.addEventListener("click", function () {
      var container = button.closest("[data-copy-container]") || button.parentElement;
      var code = container.querySelector("code");
      if (!code) {
        return;
      }
      copyText(code.textContent.replace(/\s+$/, "")).then(function () {
        button.textContent = "Copied";
        window.setTimeout(function () {
          button.textContent = "Copy";
        }, 1400);
      }).catch(function () {
        button.textContent = "Select text";
      });
    });
  });
})();
