// This tiny compile-time probe records the public Qt ABI offset used by the
// purego event filter. Run it against both majors in CI when changing Qt event
// handling; the Go tests then verify that the offset reads real wheel input.
#include <QWheelEvent>
#include <cstdio>
#include <cstddef>

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

int main() {
    WheelProbe event(QPointF(0, 0), QPointF(0, 0), QPoint(0, 0), QPoint(0, 120),
                     Qt::NoButton, Qt::NoModifier, Qt::ScrollUpdate, false);
    std::printf("Qt %d QWheelEvent angleDelta offset: %td; value: %d\n",
                QT_VERSION_MAJOR, event.angleDeltaOffset(), event.angleDelta().y());
}
