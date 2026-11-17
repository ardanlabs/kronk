package main

import tea "charm.land/bubbletea/v2"

// The conversation starts after the outer padding, title, model and divider.
const conversationTop = 4

// scrollbar returns the thumb's position and size in terminal rows. Rendering
// and hit testing use the same geometry, including soft-wrapped content.
func (m *chat) scrollbar() (int, int) {
	height, total := m.view.Height(), m.view.TotalLineCount()
	if total <= height {
		return 0, 0
	}
	thumb := max(1, height*height/total)
	top := int(float64(height-thumb)*m.view.ScrollPercent() + 0.5)
	return top, thumb
}

func (m *chat) mouse(msg tea.MouseMsg) {
	if _, ok := msg.(tea.MouseReleaseMsg); ok {
		m.dragging = false
		return
	}
	if m.width < 40 || m.height < 18 {
		return
	}
	mouse := msg.Mouse()
	height := m.view.Height()
	inside := mouse.X >= 2 && mouse.X < m.width-2 && mouse.Y >= conversationTop && mouse.Y < conversationTop+height
	top, thumb := m.scrollbar()
	switch msg.(type) {
	case tea.MouseWheelMsg:
		if inside {
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.view.ScrollUp(3)
			case tea.MouseWheelDown:
				m.view.ScrollDown(3)
			}
		}
		return
	case tea.MouseClickMsg:
		if !inside || mouse.X != m.width-3 || mouse.Button != tea.MouseLeft || thumb == 0 {
			return
		}
		m.dragging = true
		m.dragGrab = thumb / 2
		if row := mouse.Y - conversationTop; row >= top && row < top+thumb {
			m.dragGrab = row - top
			return
		}
	case tea.MouseMotionMsg:
		if !m.dragging {
			return
		}
	default:
		return
	}
	if track := height - thumb; thumb > 0 && track > 0 {
		position := max(0, min(track, mouse.Y-conversationTop-m.dragGrab))
		maxOffset := m.view.TotalLineCount() - height
		m.view.SetYOffset((position*maxOffset + track/2) / track)
	}
}
