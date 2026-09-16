(function () {
  "use strict";

  var tabs = Array.prototype.slice.call(document.querySelectorAll('[role="tab"]'));

  function selectTab(index, moveFocus) {
    tabs.forEach(function (tab, tabIndex) {
      var selected = tabIndex === index;
      var panel = document.getElementById(tab.getAttribute("aria-controls"));
      tab.setAttribute("aria-selected", selected ? "true" : "false");
      tab.tabIndex = selected ? 0 : -1;
      if (panel) {
        panel.hidden = !selected;
      }
    });
    if (moveFocus) {
      tabs[index].focus();
    }
  }

  if (tabs.length) {
    tabs.forEach(function (tab, index) {
      tab.addEventListener("click", function () {
        selectTab(index, false);
      });
      tab.addEventListener("keydown", function (event) {
        var nextIndex = null;
        if (event.key === "ArrowRight" || event.key === "ArrowDown") {
          nextIndex = index === tabs.length - 1 ? 0 : index + 1;
        } else if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
          nextIndex = index === 0 ? tabs.length - 1 : index - 1;
        } else if (event.key === "Home") {
          nextIndex = 0;
        } else if (event.key === "End") {
          nextIndex = tabs.length - 1;
        }
        if (nextIndex !== null) {
          event.preventDefault();
          selectTab(nextIndex, true);
        }
      });
    });
    selectTab(0, false);
  }

  var reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var revealGroups = document.querySelectorAll(".reveal-group");
  if (!reduceMotion && "IntersectionObserver" in window) {
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) {
          return;
        }
        entry.target.classList.remove("is-reveal-pending");
        entry.target.classList.add("is-revealed");
        observer.unobserve(entry.target);
      });
    }, { threshold: 0.08 });

    Array.prototype.forEach.call(revealGroups, function (group) {
      group.classList.add("is-reveal-pending");
      observer.observe(group);
    });
  }
})();
