import asyncio
import json
import urllib.request
import urllib.error
import decky

HUD_URL = "http://127.0.0.1:6626/api/deck/hud/state"

class Plugin:
    async def _main(self):
        self.running = True
        self.version = 0
        self.failures = 0
        self.task = asyncio.create_task(self._hud_loop())
        decky.logger.info("JACoB HUD Bridge started")

    async def _unload(self):
        self.running = False
        if getattr(self, "task", None):
            self.task.cancel()
            try:
                await self.task
            except BaseException:
                pass

    async def _fetch(self):
        url = f"{HUD_URL}?since={self.version}&wait=1000"
        def do_fetch():
            req = urllib.request.Request(url, headers={"Cache-Control": "no-store"})
            with urllib.request.urlopen(req, timeout=2.0) as response:
                return json.loads(response.read().decode("utf-8"))
        return await asyncio.to_thread(do_fetch)

    async def _hud_loop(self):
        while self.running:
            try:
                snap = await self._fetch()
                self.failures = 0
                self.version = int(snap.get("version", self.version))
                snap["connected"] = True
                await decky.emit("jacob_hud_snapshot", snap)
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                self.failures += 1
                if self.failures == 1 or self.failures % 10 == 0:
                    decky.logger.debug(f"JACoB HUD bridge waiting for core: {exc}")
                if self.failures >= 3:
                    await decky.emit("jacob_hud_snapshot", {
                        "hudVersion": 2,
                        "version": self.version,
                        "visible": False,
                        "layers": {},
                        "connected": False
                    })
                await asyncio.sleep(0.5)
