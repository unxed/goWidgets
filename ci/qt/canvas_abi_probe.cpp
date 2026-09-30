// This tiny compile-time probe records the public Qt ABI offset used by the
// purego event filter. Run it against both majors in CI when changing Qt event
// handling; the Go tests then verify that the offset reads real wheel input.
#include <QWheelEvent>
#include <QMouseEvent>
#include <cstdio>
#include <cstddef>
#include <cstring>

class WheelProbe : public QWheelEvent {
public:
    using QWheelEvent::QWheelEvent;

    std::ptrdiff_t angleDeltaOffset() const {
#if QT_VERSION >= QT_VERSION_CHECK(6, 0, 0)
        return reinterpret_cast<const char *>(&m_angleDelta) - reinterpret_cast<const char *>(this);
#else
        return reinterpret_cast<const char *>(&angleD) - reinterpret_cast<const char *>(this);
#endif
    }
};

class MouseProbe : public QMouseEvent {
public:
#if QT_VERSION >= QT_VERSION_CHECK(6, 0, 0)
    MouseProbe() : QMouseEvent(QEvent::MouseButtonPress, QPointF(0, 0), QPointF(0, 0), QPointF(0, 0),
                               Qt::MiddleButton, Qt::MouseButtons(Qt::LeftButton | Qt::MiddleButton), Qt::ShiftModifier) {}
#else
    MouseProbe() : QMouseEvent(QEvent::MouseButtonPress, QPointF(0, 0), Qt::MiddleButton,
                               Qt::MouseButtons(Qt::LeftButton | Qt::MiddleButton), Qt::ShiftModifier) {}
#endif

    std::ptrdiff_t buttonPairOffset() const {
        const auto *bytes = reinterpret_cast<const char *>(this);
        for (std::ptrdiff_t i = 0; i + 2 * sizeof(int) <= sizeof(*this); i += alignof(int)) {
            int button = 0, buttons = 0;
            std::memcpy(&button, bytes + i, sizeof(button));
            std::memcpy(&buttons, bytes + i + sizeof(button), sizeof(buttons));
            if (button == int(Qt::MiddleButton) && buttons == int(Qt::LeftButton | Qt::MiddleButton))
                return i;
        }
        return -1;
    }
};

int main() {
    WheelProbe event(QPointF(0, 0), QPointF(0, 0), QPoint(0, 0), QPoint(0, 120),
                     Qt::NoButton, Qt::NoModifier, Qt::ScrollUpdate, false);
    std::printf("Qt %d QWheelEvent angleDelta offset: %td; value: %d\n",
                QT_VERSION_MAJOR, event.angleDeltaOffset(), event.angleDelta().y());
    MouseProbe mouse;
    std::printf("Qt %d QMouseEvent button/buttons offset: %td\n",
                QT_VERSION_MAJOR, mouse.buttonPairOffset());
}
