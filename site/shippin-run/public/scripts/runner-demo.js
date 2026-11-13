const runners = [
  { name: "linux-x64-2", x: 180, state: "ok" },
  { name: "linux-arm-2", x: 420, state: "ok" },
  { name: "macos-m4", x: 660, state: "warn" },
];

export function mount(stage, options) {
  const canvas = document.createElement("canvas");
  canvas.width = 960;
  canvas.height = 540;
  canvas.className = "demo-canvas";
  canvas.setAttribute("role", "img");
  canvas.setAttribute("aria-label", "Simulated runner rack. Three labeled runners. This demo does not talk to a live fleet.");
  stage.appendChild(canvas);
  const live = stage.querySelector("[data-demo-live]");
  const queue = document.createElement("button");
  queue.type = "button";
  queue.className = "button";
  queue.textContent = "Queue a job";
  stage.appendChild(queue);
  const ctx = canvas.getContext("2d", { alpha: false });
  let job = null;
  const draw = (t) => {
    ctx.fillStyle = "#14180f";
    ctx.fillRect(0, 0, 960, 540);
    ctx.fillStyle = "#2c3824";
    ctx.fillRect(80, 390, 800, 4);
    runners.forEach((runner) => {
      ctx.save();
      ctx.translate(runner.x, 250);
      ctx.transform(1, 0.35, -0.7, 0.55, 0, 0);
      ctx.fillStyle = runner.state === "warn" ? "#e0a15a" : "#d6ef4a";
      ctx.fillRect(-70, -36, 140, 72);
      ctx.restore();
      ctx.fillStyle = "#e7ecd8";
      ctx.font = "600 22px sans-serif";
      ctx.fillText(runner.name, runner.x - 70, 430);
      ctx.fillStyle = "#a8b49a";
      ctx.font = "18px sans-serif";
      ctx.fillText(runner.state, runner.x - 70, 458);
    });
    if (job) {
      const p = Math.min(1, (t - job.start) / 900);
      const x = 80 + (runners[job.index].x - 80) * p;
      const y = 120 + 140 * p;
      ctx.fillStyle = "#9dcf7a";
      ctx.fillRect(x, y, 18, 18);
      if (p >= 1) {
        if (live) live.textContent = `Job landed on ${runners[job.index].name}. Log line: token=«redacted». This is a demo.`;
        job = null;
      }
    }
    if (job && !options.reduce) requestAnimationFrame(draw);
  };
  const frame = (now) => draw(now || 0);
  frame(0);
  queue.addEventListener("click", () => {
    const index = Math.floor(Math.random() * runners.length);
    job = { index, start: performance.now() };
    if (live) live.textContent = `Queued for ${runners[index].name}.`;
    if (options.reduce) {
      draw(job.start + 1000);
      return;
    }
    requestAnimationFrame(draw);
  });
}
