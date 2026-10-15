#define _GNU_SOURCE
#include <errno.h>
#include <inttypes.h>
#include <stdbool.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include <wayland-client.h>
#include <pipewire/pipewire.h>
#include <spa/param/video/format-utils.h>
#include <spa/param/buffers.h>
#include <spa/utils/result.h>

#define JACOB_MAGIC 0x3152464aU /* "JFR1" little-endian */
#define JACOB_DEFAULT_APPID 359320U
#define JACOB_DEFAULT_WIDTH 1280U
#define JACOB_DEFAULT_HEIGHT 800U
#define JACOB_DEFAULT_FPS 10U
#define JACOB_MAX_BUFFERS 8

#define SPA_FORMAT_VIDEO_requested_size 0x70000
#define SPA_FORMAT_VIDEO_gamescope_focus_appid 0x70001

struct gamescope_pipewire;

static const struct wl_message jacob_gamescope_pipewire_requests[] = {
    { "destroy", "", NULL },
};
static const struct wl_message jacob_gamescope_pipewire_events[] = {
    { "stream_node", "u", NULL },
};
static const struct wl_interface jacob_gamescope_pipewire_interface = {
    "gamescope_pipewire", 1,
    1, jacob_gamescope_pipewire_requests,
    1, jacob_gamescope_pipewire_events
};

struct jacob_gamescope_pipewire_listener {
    void (*stream_node)(void *data, struct gamescope_pipewire *obj, uint32_t node_id);
};

struct jacob_data {
    struct pw_main_loop *loop;
    struct pw_stream *stream;
    struct spa_hook stream_listener;
    struct spa_video_info_raw format;
    uint32_t node_id;
    uint32_t appid;
    uint32_t requested_width;
    uint32_t requested_height;
    uint32_t fps;
    uint64_t sequence;
    int64_t last_emit_ns;
    bool stream_error;
};

struct jacob_frame_header {
    uint32_t magic;
    uint32_t width;
    uint32_t height;
    uint32_t payload_bytes;
    uint32_t sequence;
};

static int64_t mono_ns(void) {
    struct timespec ts;
    if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0) return 0;
    return (int64_t)ts.tv_sec * 1000000000LL + ts.tv_nsec;
}

static int write_all(int fd, const void *buf, size_t len) {
    const uint8_t *p = (const uint8_t*)buf;
    while (len > 0) {
        ssize_t n = write(fd, p, len);
        if (n > 0) {
            p += n;
            len -= (size_t)n;
            continue;
        }
        if (n < 0 && errno == EINTR) continue;
        return -1;
    }
    return 0;
}

static void gs_stream_node(void *userdata, struct gamescope_pipewire *obj, uint32_t node_id) {
    (void)obj;
    struct jacob_data *data = userdata;
    data->node_id = node_id;
}

static const struct jacob_gamescope_pipewire_listener gs_listener = {
    .stream_node = gs_stream_node,
};

static void registry_global(
    void *userdata, struct wl_registry *registry, uint32_t name,
    const char *interface, uint32_t version) {
    struct jacob_data *data = userdata;
    if (data->node_id != 0) return;
    if (strcmp(interface, jacob_gamescope_pipewire_interface.name) != 0) return;
    uint32_t bind_version = version < 1 ? version : 1;
    if (bind_version < 1) return;
    struct wl_proxy *proxy = (struct wl_proxy*)wl_registry_bind(
        registry, name, &jacob_gamescope_pipewire_interface, bind_version);
    if (!proxy) return;
    wl_proxy_add_listener(
        proxy, (void (**)(void))&gs_listener, data);
}

static void registry_remove(void *userdata, struct wl_registry *registry, uint32_t name) {
    (void)userdata; (void)registry; (void)name;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_remove,
};

static uint32_t discover_gamescope_node(struct jacob_data *data, const char *socket_name) {
    struct wl_display *display = wl_display_connect(
        socket_name && *socket_name ? socket_name : "gamescope-0");
    if (!display) {
        fprintf(stderr, "jacob-gamescope-stream: cannot connect to Gamescope Wayland socket %s\n",
                socket_name && *socket_name ? socket_name : "gamescope-0");
        return 0;
    }
    struct wl_registry *registry = wl_display_get_registry(display);
    if (!registry) {
        wl_display_disconnect(display);
        return 0;
    }
    wl_registry_add_listener(registry, &registry_listener, data);
    wl_display_roundtrip(display);
    wl_display_roundtrip(display);
    wl_registry_destroy(registry);
    wl_display_disconnect(display);
    return data->node_id;
}

static void stream_state_changed(
    void *userdata, enum pw_stream_state old_state,
    enum pw_stream_state state, const char *error) {
    (void)old_state;
    struct jacob_data *data = userdata;
    if (state == PW_STREAM_STATE_ERROR) {
        data->stream_error = true;
        fprintf(stderr, "jacob-gamescope-stream: PipeWire stream error: %s\n",
                error ? error : "unknown");
        pw_main_loop_quit(data->loop);
    } else if (state == PW_STREAM_STATE_UNCONNECTED && old_state != PW_STREAM_STATE_UNCONNECTED) {
        pw_main_loop_quit(data->loop);
    }
}

static void stream_param_changed(void *userdata, uint32_t id, const struct spa_pod *param) {
    struct jacob_data *data = userdata;
    if (param == NULL || id != SPA_PARAM_Format) return;

    struct spa_video_info_raw info = {0};
    if (spa_format_video_raw_parse(param, &info) < 0) {
        pw_stream_set_error(data->stream, -EINVAL, "could not parse raw video format");
        return;
    }
    if (info.format != SPA_VIDEO_FORMAT_BGRx) {
        pw_stream_set_error(data->stream, -EINVAL, "unexpected video format %u", info.format);
        return;
    }
    if (info.size.width == 0 || info.size.height == 0) {
        pw_stream_set_error(data->stream, -EINVAL, "invalid video size");
        return;
    }
    data->format = info;

    int stride = (int)info.size.width * 4;
    int size = stride * (int)info.size.height;
    uint8_t buffer[512];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));
    const struct spa_pod *params[1];

    params[0] = spa_pod_builder_add_object(
        &b,
        SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
        SPA_PARAM_BUFFERS_buffers, SPA_POD_CHOICE_RANGE_Int(4, 2, JACOB_MAX_BUFFERS),
        SPA_PARAM_BUFFERS_blocks, SPA_POD_Int(1),
        SPA_PARAM_BUFFERS_size, SPA_POD_Int(size),
        SPA_PARAM_BUFFERS_stride, SPA_POD_Int(stride),
        SPA_PARAM_BUFFERS_dataType, SPA_POD_CHOICE_FLAGS_Int(
            (1 << SPA_DATA_MemFd)));

    pw_stream_update_params(data->stream, params, 1);
}

static void emit_frame(struct jacob_data *data, struct spa_buffer *buf) {
    if (!buf || buf->n_datas < 1) return;
    struct spa_data *sd = &buf->datas[0];
    if (!sd->data || !sd->chunk) return;

    const uint32_t width = data->format.size.width;
    const uint32_t height = data->format.size.height;
    if (width == 0 || height == 0) return;

    int32_t stride = sd->chunk->stride;
    if (stride <= 0) stride = (int32_t)width * 4;
    uint32_t offset = sd->chunk->offset;

    int64_t now = mono_ns();
    int64_t interval = 1000000000LL / (data->fps ? data->fps : JACOB_DEFAULT_FPS);
    if (data->last_emit_ns != 0 && now - data->last_emit_ns < interval) return;
    data->last_emit_ns = now;

    const uint32_t payload = width * height * 4;
    struct jacob_frame_header hdr = {
        .magic = JACOB_MAGIC,
        .width = width,
        .height = height,
        .payload_bytes = payload,
        .sequence = (uint32_t)(++data->sequence),
    };

    if (write_all(STDOUT_FILENO, &hdr, sizeof(hdr)) != 0) {
        pw_main_loop_quit(data->loop);
        return;
    }

    uint8_t *row = malloc((size_t)width * 4);
    if (!row) {
        pw_main_loop_quit(data->loop);
        return;
    }

    const uint8_t *base = (const uint8_t*)sd->data + offset;
    for (uint32_t y = 0; y < height; y++) {
        const uint8_t *src = base + (size_t)y * (size_t)stride;
        for (uint32_t x = 0; x < width; x++) {
            const uint8_t *p = src + (size_t)x * 4;
            uint8_t *q = row + (size_t)x * 4;
            q[0] = p[2];
            q[1] = p[1];
            q[2] = p[0];
            q[3] = 255;
        }
        if (write_all(STDOUT_FILENO, row, (size_t)width * 4) != 0) {
            free(row);
            pw_main_loop_quit(data->loop);
            return;
        }
    }
    free(row);
}

static void stream_process(void *userdata) {
    struct jacob_data *data = userdata;
    struct pw_buffer *latest = NULL;

    while (true) {
        struct pw_buffer *b = pw_stream_dequeue_buffer(data->stream);
        if (!b) break;
        if (latest) pw_stream_queue_buffer(data->stream, latest);
        latest = b;
    }
    if (!latest) return;

    emit_frame(data, latest->buffer);
    pw_stream_queue_buffer(data->stream, latest);
}

static const struct pw_stream_events stream_events = {
    .version = PW_VERSION_STREAM_EVENTS,
    .state_changed = stream_state_changed,
    .param_changed = stream_param_changed,
    .process = stream_process,
};

static const struct spa_pod *build_format(struct jacob_data *data, struct spa_pod_builder *b) {
    struct spa_rectangle size = SPA_RECTANGLE(
        data->requested_width, data->requested_height);
    struct spa_fraction rate = SPA_FRACTION(
        data->fps ? data->fps : JACOB_DEFAULT_FPS, 1);

    return spa_pod_builder_add_object(
        b,
        SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
        SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_video),
        SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
        SPA_FORMAT_VIDEO_format, SPA_POD_Id(SPA_VIDEO_FORMAT_BGRx),
        SPA_FORMAT_VIDEO_size, SPA_POD_Rectangle(&size),
        SPA_FORMAT_VIDEO_framerate, SPA_POD_Fraction(&rate),
        SPA_FORMAT_VIDEO_requested_size, SPA_POD_Rectangle(&size),
        SPA_FORMAT_VIDEO_gamescope_focus_appid,
            SPA_POD_Long((int64_t)data->appid));
}

static void quit_signal(void *userdata, int signum) {
    (void)signum;
    struct jacob_data *data = userdata;
    if (data && data->loop) pw_main_loop_quit(data->loop);
}

static uint32_t parse_u32(const char *s, uint32_t fallback) {
    if (!s || !*s) return fallback;
    char *end = NULL;
    unsigned long v = strtoul(s, &end, 10);
    if (!end || *end != '\0' || v == 0 || v > UINT32_MAX) return fallback;
    return (uint32_t)v;
}

int main(int argc, char **argv) {
    signal(SIGPIPE, SIG_IGN);

    struct jacob_data data = {0};
    data.appid = parse_u32(argc > 1 ? argv[1] : NULL, JACOB_DEFAULT_APPID);
    data.requested_width = parse_u32(argc > 2 ? argv[2] : NULL, JACOB_DEFAULT_WIDTH);
    data.requested_height = parse_u32(argc > 3 ? argv[3] : NULL, JACOB_DEFAULT_HEIGHT);
    data.fps = parse_u32(argc > 4 ? argv[4] : NULL, JACOB_DEFAULT_FPS);
    if (data.fps > 30) data.fps = 30;
    const char *socket_name = argc > 5 ? argv[5] : "gamescope-0";

    if (discover_gamescope_node(&data, socket_name) == 0) {
        return 2;
    }

    pw_init(&argc, &argv);
    data.loop = pw_main_loop_new(NULL);
    if (!data.loop) return 3;

    pw_loop_add_signal(pw_main_loop_get_loop(data.loop), SIGINT, quit_signal, &data);
    pw_loop_add_signal(pw_main_loop_get_loop(data.loop), SIGTERM, quit_signal, &data);

    struct pw_properties *props = pw_properties_new(
        PW_KEY_MEDIA_TYPE, "Video",
        PW_KEY_MEDIA_CATEGORY, "Capture",
        PW_KEY_MEDIA_ROLE, "Camera",
        PW_KEY_APP_NAME, "JACoB Gamescope Capture",
        NULL);
    if (!props) {
        pw_main_loop_destroy(data.loop);
        pw_deinit();
        return 4;
    }

    data.stream = pw_stream_new_simple(
        pw_main_loop_get_loop(data.loop),
        "jacob-gamescope-stream",
        props,
        &stream_events,
        &data);
    if (!data.stream) {
        pw_main_loop_destroy(data.loop);
        pw_deinit();
        return 5;
    }

    uint8_t podbuf[1024];
    struct spa_pod_builder builder = SPA_POD_BUILDER_INIT(podbuf, sizeof(podbuf));
    const struct spa_pod *params[1];
    params[0] = build_format(&data, &builder);

    int res = pw_stream_connect(
        data.stream,
        PW_DIRECTION_INPUT,
        data.node_id,
        PW_STREAM_FLAG_AUTOCONNECT | PW_STREAM_FLAG_MAP_BUFFERS,
        params, 1);
    if (res < 0) {
        fprintf(stderr, "jacob-gamescope-stream: connect failed: %s\n", spa_strerror(res));
        pw_stream_destroy(data.stream);
        pw_main_loop_destroy(data.loop);
        pw_deinit();
        return 6;
    }

    fprintf(stderr,
        "jacob-gamescope-stream: node=%u appid=%u requested=%ux%u fps=%u\n",
        data.node_id, data.appid, data.requested_width, data.requested_height, data.fps);

    pw_main_loop_run(data.loop);

    pw_stream_destroy(data.stream);
    pw_main_loop_destroy(data.loop);
    pw_deinit();
    return data.stream_error ? 7 : 0;
}
