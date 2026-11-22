const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

document.querySelectorAll("[data-demo]").forEach((button) => {
  button.addEventListener("click", async () => {
    const stage = document.querySelector("[data-demo-stage]");
    if (!stage || stage.dataset.ready === "1") return;
    stage.dataset.ready = "1";
    button.setAttribute("aria-busy", "true");
    const mod = await import("./runner-demo.js");
    mod.mount(stage, { reduce });
    button.hidden = true;
  });
});
