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

  // Every docs code block gets the same copy control the setup prompt has.
  Array.prototype.forEach.call(document.querySelectorAll(".docs-content pre"), function (pre) {
    if (pre.closest("[data-copy-container]") || !pre.querySelector("code")) {
      return;
    }
    var block = document.createElement("div");
    block.className = "code-block";
    block.setAttribute("data-copy-container", "");
    pre.parentNode.insertBefore(block, pre);
    block.appendChild(pre);
    var button = document.createElement("button");
    button.type = "button";
    button.className = "code-copy";
    button.setAttribute("data-copy", "");
    button.setAttribute("aria-label", "Copy code");
    button.textContent = "Copy";
    block.appendChild(button);
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
