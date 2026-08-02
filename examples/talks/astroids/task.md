### Browser Asteroids Game

Implement a small, complete Asteroids-style browser game served by a Go web service and use the `writing-go` skill for
the Go parts.

Keep the implementation focused. The game needs only:

- The player's ship
- Asteroids
- Bullets fired by the ship
- Three lives
- A score
- Game-over and restart behavior

Do not add levels, power-ups, enemy ships, bosses, sound, music, particle systems, mobile controls, networking, accounts, persistence, configuration, or other unrequested features.

Start coding immediately without questions or a plan.

## Architecture

Create the application under `asteroids/` with this structure:

```text
asteroids/
├── main.go
└── static/
    ├── index.html
    ├── styles.css
    └── game.js
```

Use only:

- The Go standard library for the web service
- Plain HTML
- Plain CSS
- Plain browser JavaScript
- The HTML `<canvas>` element for gameplay rendering

Do not use external packages, JavaScript frameworks, CSS frameworks, image assets, fonts, CDNs, or third-party libraries.

Do not use htmx. It is unnecessary because all game state, input handling, physics, collision detection, rendering, scoring, and restart behavior must run entirely in the browser. The Go service only serves the embedded static files and must not participate in the game loop.

## Go web service

Implement `asteroids/main.go` with these requirements:

- Embed the `static` directory into the executable using `embed`.
- Use `fs.Sub` and `http.FileServer` to serve the embedded files.
- Serve the application at `http://localhost:8080/`.
- Use only the Go standard library.
- Log the address when the service starts.
- Return an error from the server startup rather than silently ignoring it.
- Do not implement APIs, WebSockets, server-side game state, templates, or persistence.

The application must continue working when built as a standalone executable outside the source directory. It must not depend on static files remaining on disk at runtime.

## HTML

Implement `static/index.html` as a minimal, valid HTML document that:

- Loads `styles.css`.
- Loads `game.js` with `defer`.
- Contains a heading for the game.
- Contains a HUD showing:
  - `Score: 0`
  - `Lives: 3`
- Contains one canvas with a logical size of exactly `960 × 640`.
- Contains a short controls line:
  - `W or ↑: Thrust`
  - `A/D or ←/→: Rotate`
  - `Space: Fire`
- Contains an initially hidden game-over overlay or message.
- Does not contain inline JavaScript or inline CSS.

Use accessible text for the canvas and game-over state, but do not build additional menus or settings.

## Visual design

Keep the visuals simple and recognizable:

- Page background: very dark or black.
- Canvas background: black.
- Ship: white outlined triangle.
- Asteroids: irregular white or light-gray outlined polygons.
- Bullets: small solid white circles.
- HUD and controls: readable light-colored text.
- Use no image assets.
- Keep the canvas centered on the page.
- Scale the canvas down with CSS when the browser viewport is narrower than 960 pixels while preserving its `3:2` aspect ratio.
- Keep the canvas's internal coordinate system fixed at `960 × 640`; CSS scaling must not change the gameplay coordinates.
- Do not add decorative panels, animations unrelated to gameplay, or elaborate visual effects.

## Game state

Store all game state in JavaScript memory.

At minimum, represent:

- The ship:
  - Position
  - Velocity
  - Facing angle
  - Collision radius
  - Temporary invulnerability state
- Each asteroid:
  - Position
  - Velocity
  - Rotation
  - Rotation speed
  - Collision radius
  - A stable set of polygon vertices generated when the asteroid is created
- Each bullet:
  - Position
  - Velocity
  - Remaining lifetime
  - Collision radius
- Current score
- Remaining lives
- Current input state
- Whether the game is running or over
- Previous animation-frame timestamp
- Bullet firing cooldown

Use clear constants near the top of `game.js` for gameplay values. Do not scatter unexplained numeric values throughout the game loop.

## Initial game state

When the page first loads:

- Start the game immediately.
- Set the score to `0`.
- Give the player exactly `3` lives.
- Place the ship in the center of the canvas.
- Point the ship upward.
- Give the ship zero initial velocity.
- Create exactly `6` asteroids.
- Do not place an asteroid within `180` pixels of the ship's initial position.
- Hide the game-over message.
- Give the ship two seconds of spawn invulnerability so an asteroid cannot immediately remove a life.

## Controls

Use keyboard controls:

- `W` or `ArrowUp`: apply forward thrust.
- `A` or `ArrowLeft`: rotate counterclockwise.
- `D` or `ArrowRight`: rotate clockwise.
- `Space`: fire.
- `Enter`: restart only after game over.

Input behavior:

- Track held keys with `keydown` and `keyup`.
- Movement and rotation must remain smooth while keys are held.
- Prevent the browser's default scrolling behavior for the arrow keys and Space while the game has focus.
- Ignore key-repeat as a source of gameplay timing; continuous actions must be driven by held-key state in the animation loop.
- Clear held-key state when the browser window loses focus so controls cannot become stuck.
- Do not install a separate animation loop from inside an input event.

## Ship movement

The ship must have arcade-style inertia so movement has a sense of momentum:

- The ship rotates independently of its current travel direction.
- Thrust accelerates it in the direction its nose is pointing.
- Releasing thrust does not stop it immediately.
- Apply mild frame-rate-independent drag so it gradually slows.
- Limit its maximum speed.
- Do not apply downward gravity.
- Do not allow reverse thrust.

Use these target values unless a small adjustment is needed for correctness:

- Rotation speed: approximately `4 radians per second`
- Forward acceleration: approximately `220 pixels per second squared`
- Maximum ship speed: approximately `350 pixels per second`
- Drag coefficient: approximately `0.25 per second`

Make movement frame-rate independent by multiplying acceleration, rotation, velocity changes, and timers by elapsed time in seconds.

## World wrapping

The game world wraps at all four canvas edges:

- The ship wraps from one side to the opposite side.
- Asteroids wrap from one side to the opposite side.
- Bullets wrap from one side to the opposite side while they remain alive.
- Account for each object's radius when wrapping so objects do not visibly disappear too early.
- Preserve velocity and angle when wrapping.

Use toroidal or wrap-aware distance for collision checks so collisions near opposite edges behave consistently.

## Shooting

The ship fires bullets from its nose, not from its center.

When firing:

- Compute the nose position from the ship's current position, facing angle, and visible ship length.
- Create the bullet at that nose position.
- Send it in the exact direction the ship is facing.
- Add the ship's current velocity to the bullet's forward velocity so bullets inherit the ship's momentum.
- Use a bullet speed of approximately `500 pixels per second` relative to the ship.
- Give each bullet a lifetime of approximately `1.2 seconds`.
- Remove a bullet after its lifetime expires.
- Allow Space to be held to fire repeatedly.
- Enforce a firing cooldown of approximately `0.2 seconds`.
- Do not fire while the game is over.

The bullet must visibly emerge from the point of the triangular ship.

## Asteroids

Create exactly six asteroids whenever a set begins.

Each asteroid must:

- Be an irregular outlined polygon, not a perfect circle.
- Keep the same polygon shape across frames rather than regenerating random vertices during rendering.
- Have between `8` and `12` vertices.
- Have a collision radius between approximately `28` and `48` pixels.
- Move at a random speed between approximately `35` and `90` pixels per second.
- Move in a random direction.
- Rotate slowly at a random clockwise or counterclockwise rate.
- Wrap around the canvas edges.

Asteroids do not split. Destroying an asteroid removes it completely and adds `100` points to the score.

When all six asteroids have been destroyed, create another set of exactly six asteroids after a short delay or on the following frame. Keep the same asteroid count and difficulty. Do not display a level or wave number.

New asteroids must not spawn within `180` pixels of the ship. Use a bounded retry loop with a safe fallback so asteroid generation cannot hang forever.

## Collision detection

Use circle-based collision detection for gameplay even though the ship and asteroids are drawn as polygons.

A collision occurs when the wrap-aware distance between two object centers is less than the sum of their collision radii.

Check these collisions after movement updates:

### Bullet against asteroid

When a bullet hits an asteroid:

- Remove that bullet.
- Remove that asteroid.
- Add exactly `100` points to the score.
- Update the visible score immediately.
- A single bullet can destroy at most one asteroid.
- Do not split the asteroid.
- Do not create particle effects.

Iterate or remove objects safely so array mutation does not skip later objects or access removed entries.

### Ship against asteroid

When a non-invulnerable ship touches an asteroid:

- Remove one life.
- Update the visible lives immediately.
- Do not remove the asteroid.
- Clear all active bullets.
- If lives remain:
  - Reset the ship to the center.
  - Reset its velocity to zero.
  - Point it upward.
  - Give it two seconds of invulnerability.
- If no lives remain:
  - Set lives to `0`.
  - Stop gameplay updates.
  - Enter the game-over state.

During spawn invulnerability:

- Ignore ship-versus-asteroid collisions.
- Make the ship blink at a steady rate so invulnerability is visible.
- Continue allowing movement and firing.
- End invulnerability after two seconds.

Resolve at most one ship collision per frame.

## Score and lives

The HUD must always reflect current state:

```text
Score: 0
Lives: 3
```

Rules:

- Begin with `0` points.
- Add `100` points per destroyed asteroid.
- Begin with exactly `3` lives.
- Remove one life per valid ship collision.
- Never display a negative life count.
- Restarting after game over resets the score to `0` and lives to `3`.

Do not use browser storage. Score and lives do not persist after refreshing or leaving the page.

## Game loop

Use exactly one `requestAnimationFrame` loop.

For each frame:

1. Compute elapsed time in seconds from the animation-frame timestamp.
2. Cap elapsed time at approximately `0.05` seconds so returning to a backgrounded tab does not cause a huge physics jump.
3. Read held-key state.
4. Rotate and accelerate the ship.
5. Apply frame-rate-independent drag and the speed limit.
6. Apply bullet cooldown and create bullets when appropriate.
7. Move the ship, bullets, and asteroids.
8. Apply world wrapping.
9. Reduce bullet lifetimes and remove expired bullets.
10. Check bullet-versus-asteroid collisions.
11. Check ship-versus-asteroid collisions when allowed.
12. Replenish asteroids when all have been destroyed.
13. Render the complete frame.
14. Request the next animation frame.

When the game is over:

- Continue rendering the final scene.
- Do not move objects.
- Do not process movement, firing, spawning, or collisions.
- Continue requesting frames only if needed for a straightforward single-loop implementation; do not start another loop when restarting.

## Rendering

Clear and redraw the entire canvas every frame.

Render in this order:

1. Black background
2. Asteroids
3. Bullets
4. Ship
5. Game-over message, if applicable

Ship rendering:

- Draw a clearly recognizable outlined triangle.
- The triangle's nose must point in the ship's facing direction.
- Use canvas transforms with `save`, `translate`, `rotate`, and `restore`, or equivalent correct geometry.
- Ensure the visual nose position uses the same ship length used to place bullets.

Asteroid rendering:

- Draw each asteroid from its stored local polygon vertices.
- Translate and rotate the canvas for that asteroid.
- Close and stroke the path.
- Do not randomize its appearance during rendering.

All canvas transform operations must be balanced so transforms do not leak into later objects.

## Game over and restart

When lives reach zero:

- Show a centered message over the canvas containing:
  - `Game Over`
  - The final score
  - `Press Enter to restart`
- Keep the HUD visible.
- Ignore movement and fire controls.

When Enter is pressed during game over:

- Reset the score to `0`.
- Reset lives to `3`.
- Clear bullets.
- Replace all asteroids with six newly generated asteroids.
- Reset the ship to the center, facing upward, with zero velocity.
- Restore two seconds of spawn invulnerability.
- Hide the game-over message.
- Resume using the existing animation loop.
- Do not reload the page.
- Do not register duplicate event listeners.
- Do not start a second `requestAnimationFrame` loop.

Pressing Enter while the game is running must do nothing.

## Code quality constraints

- Keep the implementation short, direct, and readable.
- Use descriptive names.
- Separate update logic from rendering logic.
- Use small functions for coherent responsibilities such as:
  - Resetting the game
  - Creating asteroids
  - Handling input
  - Updating physics
  - Detecting collisions
  - Wrapping positions
  - Rendering the ship
  - Rendering asteroids
- Do not create classes or abstractions unless they materially simplify the implementation.
- Do not add a build system for the frontend.
- Do not add tests that require third-party packages.
- Do not add generated or minified files.
- Do not leave TODOs, placeholders, or incomplete behavior.
- Do not silently catch errors.
- Do not print per-frame debug output.

## Acceptance criteria

Before considering the task complete, confirm all of the following from the implementation:

- `go run ./asteroids` serves the game at `http://localhost:8080/`.
- The executable serves embedded assets without relying on the source directory.
- The browser loads the HTML, CSS, and JavaScript without external resources.
- The initial HUD reads `Score: 0` and `Lives: 3`.
- Exactly six asteroids appear initially.
- The ship begins centered, facing upward.
- The ship rotates smoothly.
- Thrust accelerates the ship in the direction of its nose.
- The ship coasts after thrust is released and slows gradually.
- The ship and asteroids wrap correctly at every edge.
- Space fires bullets from the ship's nose.
- Bullets travel in the ship's facing direction and inherit ship velocity.
- Bullets destroy asteroids and add `100` points.
- Asteroids do not split.
- Clearing all asteroids creates another set of six.
- A ship collision removes one life.
- The ship is briefly invulnerable after spawning or losing a life.
- The game ends when all three lives are lost.
- Enter restarts from zero score and three lives without reloading.
- Restarting does not create duplicate controls or animation loops.
- There are no extra gameplay systems or features.

## Verification

After implementation:

1. Format changed Go files with:

   ```sh
   gofmt -s -w asteroids/main.go
   ```

2. Run the Go checks required by the `writing-go` skill against the changed package.

3. Verify compilation with:

   ```sh
   go build -o /tmp/asteroids-game ./asteroids
   ```

4. Start the built service temporarily and verify with `curl` that:
   - `/` returns HTTP 200 and the HTML document.
   - `/styles.css` returns HTTP 200.
   - `/game.js` returns HTTP 200.

5. If a browser inspection tool is available, open the game and verify the acceptance criteria, including controls, firing, collisions, lives, scoring, game over, and restart.

6. Stop the temporary service.

7. Remove `/tmp/asteroids-game`.

Do not claim interactive gameplay was verified unless it was actually exercised in a browser. Report any verification that could not be performed.

Start coding immediately without questions or a plan.
