# Nuclear Explosion Animation 💥

## Overview
An epic ASCII art nuclear explosion animation that plays during infrastructure destruction, adding visual drama and engagement to the destroy process.

## Animation Sequence

The animation consists of **9 frames** that smoothly transition from initial blast to full mushroom cloud:

### Frame 0: Initial Flash ⚡
```
       .
      ':'
     ':'
    ':'
   .:.
```
**Color:** Bright Yellow (226)
**Duration:** ~300ms

### Frame 1: Small Blast 💥
```
      .:.
     :***:
    :*****:
   :*******:
    :*****:
     :***:
      ':'
```
**Color:** Bright Yellow (226)
**Duration:** ~300ms

### Frame 2-3: Expanding Wave 🌊
```
     .-"-.
    /\****/\
   |  \**/  |
   |   **   |
    \  **  /
     '-**-'
       ||
       ||
```
**Color:** Orange (208)
**Duration:** ~600ms total

### Frame 4-5: Rising Mushroom Start 🍄
```
     .:::::.
    : ***** :
   :  *****  :
   :  *****  :
    : ***** :
     ':***:'
       |||
      |||||
     |||||||
```
**Color:** Orange (208)
**Duration:** ~600ms total

### Frame 6-8: Full Mushroom Cloud ☁️
```
 .::::::::::.
: ********** :
: ********** :
: ********** :
: ********** :
: ********** :
: ********** :
 ':::::::::'
    |||||||
   |||||||||
  |||||||||||
 |||||||||||||
```
**Color:** Red (196)
**Duration:** Sustains until destruction completes

## Color Progression

The animation uses dynamic color transitions:

1. **Yellow (226)** - Frames 0-1: Initial flash and blast
   - Represents the intense heat and light
   - Catches attention immediately

2. **Orange (208)** - Frames 2-4: Expansion phase
   - Transition from flash to fireball
   - Shows energy spreading

3. **Red (196)** - Frames 5-8: Mushroom cloud
   - Final form of the explosion
   - Sustained throughout destruction
   - Matches the "destruction in progress" theme

## Animation Timing

```go
// Update every 100ms (10 FPS)
tickRate := time.Millisecond * 100

// Explosion frame advances every 3 ticks (300ms per frame)
if m.spinnerFrame%3 == 0 && m.explosionFrame < len(explosionFrames)-1 {
    m.explosionFrame++
}
```

**Total Animation Duration:** ~2.7 seconds to reach full mushroom cloud

## Visual Layout

```
        💥 INFRASTRUCTURE DESTRUCTION IN PROGRESS

                   .::::::::::.
                  : ********** :
                  : ********** :
                  : ********** :
                  : ********** :
                  : ********** :
                  : ********** :
                   ':::::::::'
                      |||||||
                     |||||||||
                    |||||||||||
                   |||||||||||||

               ⠋ Destroying: aws_ecs_service.app

                    Terraform Output:
    ╭────────────────────────────────────────────────────────╮
    │                                                        │
    │  aws_ecs_service.app: Destroying... [id=arn:aws...]  │
    │  ...                                                   │
    │                                                        │
    ╰────────────────────────────────────────────────────────╯

            Destroyed: 5 resources • Elapsed: 12.3s
```

## Implementation Details

### Frame Storage
```go
var explosionFrames = []string{
    // 9 carefully crafted ASCII art frames
    // Each frame builds upon the previous
    // Progressive detail from simple to complex
}
```

### State Management
```go
type destroyProgressModel struct {
    // ... other fields ...
    explosionFrame  int  // Current animation frame (0-8)
}
```

### Animation Control
```go
// Only animate during destruction phase
if m.phase == destroyDestroying {
    // Slower than spinner (every 3 ticks)
    if m.spinnerFrame%3 == 0 && m.explosionFrame < len(explosionFrames)-1 {
        m.explosionFrame++
    }
}
```

### Rendering
```go
// Show animation only during destruction
if m.phase == destroyDestroying {
    frameIndex := m.explosionFrame

    // Color based on intensity
    var explosionColor string
    if frameIndex < 2 {
        explosionColor = "226" // Yellow flash
    } else if frameIndex < 5 {
        explosionColor = "208" // Orange expansion
    } else {
        explosionColor = "196" // Red mushroom
    }

    // Render centered and colored
    explosionStyle := lipgloss.NewStyle().
        Foreground(lipgloss.Color(explosionColor)).
        Bold(true).
        Align(lipgloss.Center).
        Width(contentWidth)

    content.WriteString(explosionStyle.Render(explosionFrames[frameIndex]))
}
```

## User Experience

### Phases

1. **Initialization** - No animation
   - Shows spinner only
   - Preparing for destruction

2. **Planning** - No animation
   - Shows spinner only
   - Calculating what to destroy

3. **Destroying** - **ANIMATION ACTIVE** 🎬
   - Explosion animation plays
   - Transitions through all 9 frames
   - Color shifts from yellow → orange → red
   - Sustains final mushroom cloud frame

4. **Complete** - No animation
   - Shows success message
   - Animation ended

### Visual Impact

- **Attention-Grabbing:** The bright yellow flash immediately draws eyes
- **Dramatic:** Progressive expansion creates sense of power
- **Thematic:** Perfectly matches "nuclear destruction" metaphor
- **Centered:** Aligned perfectly with the title and status
- **Smooth:** 300ms per frame feels natural and cinematic

## Technical Notes

### Performance
- Lightweight: Just string rendering
- No complex calculations
- Frame data pre-loaded
- Minimal memory footprint

### Compatibility
- Works in all terminal emulators
- Pure ASCII characters (no special fonts needed)
- Color support via ANSI codes
- Degrades gracefully on basic terminals

### Frame Design Philosophy
1. **Start Small:** Simple dot represents initial impact
2. **Expand Outward:** Each frame grows larger
3. **Add Detail:** More asterisks and structural elements
4. **Build Height:** Vertical stem grows as cloud rises
5. **Cap Size:** Final frame fills comfortable space

## Easter Eggs 🥚

The animation includes subtle details:
- Frame 2 uses `\` to show blast wave direction
- Frame 3 adds `/|\` to show debris
- Frames 4+ show progressive cloud billowing
- Stem `|||` grows thicker as cloud rises
- Final cloud is symmetrical and balanced

## Future Enhancements

Potential additions:
- [ ] Shockwave rings around explosion
- [ ] Randomized debris particles
- [ ] Color fade-out after completion
- [ ] Sound effects (terminal bell)
- [ ] Adjustable animation speed
- [ ] Multiple explosion styles

## Testing

```bash
# Build
cd /Users/jack/mag/infrastructure/app
go build -o ../meroku

# Run and trigger destroy
./meroku
# Select "Nuke Environment"
# Complete the confirmation wizard
# Watch the explosion! 💥
```

## Credits

Inspired by:
- Classic ASCII art traditions
- Nuclear explosion physics
- Retro terminal aesthetics
- The dramatic nature of infrastructure destruction

---

*"I am become Death, the destroyer of clouds."* - J. Robert Oppenheimer (probably)
