#define VK_NO_PROTOTYPES
#include <vulkan/vulkan.h>
#include <vulkan/vk_layer.h>

#include <algorithm>
#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <memory>
#include <mutex>
#include <sstream>
#include <string>
#include <unordered_map>
#include <vector>

#include <fcntl.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#define JACOB_EXPORT __attribute__((visibility("default")))

namespace {

constexpr uint32_t kHUDMagic = 0x3244484a;      // JHD2
constexpr uint32_t kHUDVersion = 2;
constexpr uint32_t kCaptureMagic = 0x3246434a;  // JCF2
constexpr uint32_t kCaptureVersion = 2;
constexpr uint32_t kCaptureRGBA = 1;
constexpr uint32_t kCaptureBGRA = 2;
constexpr size_t kHUDHeaderBytes = 40;
constexpr size_t kCaptureHeaderBytes = 32;
constexpr size_t kMaxTransportBytes = 64u * 1024u * 1024u;

struct Vertex {
    float x;
    float y;
    float r;
    float g;
    float b;
    float a;
};
static_assert(sizeof(Vertex) == 24, "JACoB HUD vertex ABI changed");

struct InstanceState {
    VkInstance instance = VK_NULL_HANDLE;
    PFN_vkGetInstanceProcAddr nextGIPA = nullptr;
    PFN_GetPhysicalDeviceProcAddr nextGPDPA = nullptr;
    PFN_vkDestroyInstance DestroyInstance = nullptr;
    PFN_vkCreateDevice CreateDevice = nullptr;
    PFN_vkGetPhysicalDeviceMemoryProperties GetPhysicalDeviceMemoryProperties = nullptr;
};

struct DeviceState {
    VkPhysicalDevice physical = VK_NULL_HANDLE;
    VkDevice device = VK_NULL_HANDLE;
    PFN_vkGetDeviceProcAddr nextGDPA = nullptr;

    PFN_vkDestroyDevice DestroyDevice = nullptr;
    PFN_vkGetDeviceQueue GetDeviceQueue = nullptr;
    PFN_vkGetDeviceQueue2 GetDeviceQueue2 = nullptr;
    PFN_vkQueueSubmit QueueSubmit = nullptr;
    PFN_vkDeviceWaitIdle DeviceWaitIdle = nullptr;

    PFN_vkCreateSwapchainKHR CreateSwapchainKHR = nullptr;
    PFN_vkDestroySwapchainKHR DestroySwapchainKHR = nullptr;
    PFN_vkGetSwapchainImagesKHR GetSwapchainImagesKHR = nullptr;
    PFN_vkQueuePresentKHR QueuePresentKHR = nullptr;

    PFN_vkCreateCommandPool CreateCommandPool = nullptr;
    PFN_vkDestroyCommandPool DestroyCommandPool = nullptr;
    PFN_vkAllocateCommandBuffers AllocateCommandBuffers = nullptr;
    PFN_vkFreeCommandBuffers FreeCommandBuffers = nullptr;
    PFN_vkResetCommandBuffer ResetCommandBuffer = nullptr;
    PFN_vkBeginCommandBuffer BeginCommandBuffer = nullptr;
    PFN_vkEndCommandBuffer EndCommandBuffer = nullptr;

    PFN_vkCreateSemaphore CreateSemaphore = nullptr;
    PFN_vkDestroySemaphore DestroySemaphore = nullptr;
    PFN_vkCreateFence CreateFence = nullptr;
    PFN_vkDestroyFence DestroyFence = nullptr;
    PFN_vkResetFences ResetFences = nullptr;
    PFN_vkWaitForFences WaitForFences = nullptr;
    PFN_vkGetFenceStatus GetFenceStatus = nullptr;

    PFN_vkCreateBuffer CreateBuffer = nullptr;
    PFN_vkDestroyBuffer DestroyBuffer = nullptr;
    PFN_vkGetBufferMemoryRequirements GetBufferMemoryRequirements = nullptr;
    PFN_vkAllocateMemory AllocateMemory = nullptr;
    PFN_vkFreeMemory FreeMemory = nullptr;
    PFN_vkBindBufferMemory BindBufferMemory = nullptr;
    PFN_vkMapMemory MapMemory = nullptr;
    PFN_vkUnmapMemory UnmapMemory = nullptr;

    PFN_vkCreateImageView CreateImageView = nullptr;
    PFN_vkDestroyImageView DestroyImageView = nullptr;
    PFN_vkCreateRenderPass CreateRenderPass = nullptr;
    PFN_vkDestroyRenderPass DestroyRenderPass = nullptr;
    PFN_vkCreateFramebuffer CreateFramebuffer = nullptr;
    PFN_vkDestroyFramebuffer DestroyFramebuffer = nullptr;
    PFN_vkCreateShaderModule CreateShaderModule = nullptr;
    PFN_vkDestroyShaderModule DestroyShaderModule = nullptr;
    PFN_vkCreatePipelineLayout CreatePipelineLayout = nullptr;
    PFN_vkDestroyPipelineLayout DestroyPipelineLayout = nullptr;
    PFN_vkCreateGraphicsPipelines CreateGraphicsPipelines = nullptr;
    PFN_vkDestroyPipeline DestroyPipeline = nullptr;

    PFN_vkCmdPipelineBarrier CmdPipelineBarrier = nullptr;
    PFN_vkCmdCopyImageToBuffer CmdCopyImageToBuffer = nullptr;
    PFN_vkCmdBeginRenderPass CmdBeginRenderPass = nullptr;
    PFN_vkCmdEndRenderPass CmdEndRenderPass = nullptr;
    PFN_vkCmdBindPipeline CmdBindPipeline = nullptr;
    PFN_vkCmdBindVertexBuffers CmdBindVertexBuffers = nullptr;
    PFN_vkCmdDraw CmdDraw = nullptr;
};

struct FrameRes {
    VkCommandBuffer commandBuffer = VK_NULL_HANDLE;
    VkSemaphore done = VK_NULL_HANDLE;
    VkFence fence = VK_NULL_HANDLE;
    bool inFlight = false;

    VkBuffer vertexBuffer = VK_NULL_HANDLE;
    VkDeviceMemory vertexMemory = VK_NULL_HANDLE;
    void* vertexMapped = nullptr;
    VkDeviceSize vertexCapacity = 0;
    uint64_t generation = 0;
    uint32_t vertexCount = 0;
};

struct SwapState {
    VkSwapchainKHR swapchain = VK_NULL_HANDLE;
    VkFormat format = VK_FORMAT_UNDEFINED;
    VkExtent2D extent{};
    bool colorAttachment = false;
    bool transferSrc = false;

    std::vector<VkImage> images;
    std::vector<VkImageView> views;
    std::vector<VkFramebuffer> framebuffers;
    uint32_t queueFamily = UINT32_MAX;

    VkRenderPass renderPass = VK_NULL_HANDLE;
    VkPipelineLayout pipelineLayout = VK_NULL_HANDLE;
    VkPipeline pipeline = VK_NULL_HANDLE;
    VkCommandPool commandPool = VK_NULL_HANDLE;
    std::vector<FrameRes> frames;

    VkBuffer captureBuffer = VK_NULL_HANDLE;
    VkDeviceMemory captureMemory = VK_NULL_HANDLE;
    void* captureMapped = nullptr;
    VkDeviceSize captureCapacity = 0;
    uint64_t captureInFlightSeq = 0;
    uint32_t captureImageIndex = UINT32_MAX;
    uint64_t captureLastCompleted = 0;

    bool ready = false;
};

struct SceneState {
    uint64_t generation = 0;
    bool visible = true;
    std::vector<Vertex> vertices;
    time_t mtimeSec = 0;
    long mtimeNSec = 0;
    off_t fileSize = 0;
};

InstanceState g_instance;
DeviceState g_device;
std::unordered_map<uintptr_t, uint32_t> g_queueFamilies;
std::unordered_map<uintptr_t, std::unique_ptr<SwapState>> g_swaps;
SceneState g_scene;
std::mutex g_mu;
std::chrono::steady_clock::time_point g_lastReadyWrite{};

uintptr_t key(VkQueue v) { return reinterpret_cast<uintptr_t>(v); }
uintptr_t key(VkSwapchainKHR v) { return reinterpret_cast<uintptr_t>(v); }

std::string home_dir() {
    const char* h = std::getenv("HOME");
    return h && *h ? std::string(h) : std::string();
}

std::string runtime_dir() {
    const char* explicitDir = std::getenv("JACOB_VK_RUNTIME_DIR");
    if (explicitDir && *explicitDir) return explicitDir;
    std::string h = home_dir();
    if (!h.empty()) return h + "/.local/share/JACoB/vulkan-runtime";
    const char* xdg = std::getenv("XDG_RUNTIME_DIR");
    std::string base;
    if (xdg && *xdg) base = xdg;
    else base = "/run/user/" + std::to_string(static_cast<unsigned long>(getuid()));
    return base + "/jacob-vk";
}

std::string hud_dir() {
    const char* p = std::getenv("JACOB_VK_HUD_DIR");
    if (p && *p) return p;
    std::string h = home_dir();
    if (h.empty()) return {};
    return h + "/.local/share/JACoB/vulkan-hud";
}

void mkdirs_runtime() {
    std::string base = runtime_dir();
    std::string parent = base.substr(0, base.find_last_of('/'));
    if (!parent.empty()) mkdir(parent.c_str(), 0700);
    mkdir(base.c_str(), 0700);
}

void ensure_log_dir() {
    std::string h = home_dir();
    if (h.empty()) return;
    std::string p1 = h + "/.local";
    std::string p2 = p1 + "/state";
    std::string p3 = p2 + "/JACoB";
    mkdir(p1.c_str(), 0755);
    mkdir(p2.c_str(), 0755);
    mkdir(p3.c_str(), 0755);
}

void log_line(const char* msg) {
    ensure_log_dir();
    std::string h = home_dir();
    if (h.empty()) return;
    std::string path = h + "/.local/state/JACoB/vulkan-hud.log";
    FILE* f = std::fopen(path.c_str(), "a");
    if (!f) return;
    std::fprintf(f, "[pid=%d] %s\n", static_cast<int>(getpid()), msg);
    std::fclose(f);
}

bool write_all(int fd, const uint8_t* data, size_t n) {
    while (n > 0) {
        ssize_t w = ::write(fd, data, n);
        if (w < 0) return false;
        data += static_cast<size_t>(w);
        n -= static_cast<size_t>(w);
    }
    return true;
}

bool atomic_write(const std::string& path, const uint8_t* data, size_t size) {
    mkdirs_runtime();
    std::string tmp = path + ".tmp." + std::to_string(static_cast<int>(getpid()));
    int fd = ::open(tmp.c_str(), O_WRONLY | O_CREAT | O_TRUNC, 0600);
    if (fd < 0) return false;
    bool ok = write_all(fd, data, size);
    ::close(fd);
    if (!ok || ::rename(tmp.c_str(), path.c_str()) != 0) {
        ::unlink(tmp.c_str());
        return false;
    }
    return true;
}

bool atomic_write_text(const std::string& path, const std::string& s) {
    return atomic_write(path, reinterpret_cast<const uint8_t*>(s.data()), s.size());
}

uint32_t u32le(const uint8_t* p) {
    return static_cast<uint32_t>(p[0]) |
           (static_cast<uint32_t>(p[1]) << 8) |
           (static_cast<uint32_t>(p[2]) << 16) |
           (static_cast<uint32_t>(p[3]) << 24);
}

uint64_t u64le(const uint8_t* p) {
    return static_cast<uint64_t>(u32le(p)) |
           (static_cast<uint64_t>(u32le(p + 4)) << 32);
}

void put_u32le(uint8_t* p, uint32_t v) {
    p[0] = static_cast<uint8_t>(v);
    p[1] = static_cast<uint8_t>(v >> 8);
    p[2] = static_cast<uint8_t>(v >> 16);
    p[3] = static_cast<uint8_t>(v >> 24);
}

void put_u64le(uint8_t* p, uint64_t v) {
    put_u32le(p, static_cast<uint32_t>(v));
    put_u32le(p + 4, static_cast<uint32_t>(v >> 32));
}

template <typename T>
T load_dev(const char* name) {
    return reinterpret_cast<T>(g_device.nextGDPA(g_device.device, name));
}

VkLayerInstanceCreateInfo* instance_chain(const VkInstanceCreateInfo* ci, VkLayerFunction fn) {
    struct Header { VkStructureType sType; const void* pNext; };
    auto* h = reinterpret_cast<const Header*>(ci ? ci->pNext : nullptr);
    while (h) {
        if (h->sType == VK_STRUCTURE_TYPE_LOADER_INSTANCE_CREATE_INFO) {
            auto* li = reinterpret_cast<VkLayerInstanceCreateInfo*>(const_cast<Header*>(h));
            if (li->function == fn) return li;
        }
        h = reinterpret_cast<const Header*>(h->pNext);
    }
    return nullptr;
}

VkLayerDeviceCreateInfo* device_chain(const VkDeviceCreateInfo* ci, VkLayerFunction fn) {
    struct Header { VkStructureType sType; const void* pNext; };
    auto* h = reinterpret_cast<const Header*>(ci ? ci->pNext : nullptr);
    while (h) {
        if (h->sType == VK_STRUCTURE_TYPE_LOADER_DEVICE_CREATE_INFO) {
            auto* li = reinterpret_cast<VkLayerDeviceCreateInfo*>(const_cast<Header*>(h));
            if (li->function == fn) return li;
        }
        h = reinterpret_cast<const Header*>(h->pNext);
    }
    return nullptr;
}

bool supported_format(VkFormat f) {
    return f == VK_FORMAT_B8G8R8A8_UNORM ||
           f == VK_FORMAT_B8G8R8A8_SRGB ||
           f == VK_FORMAT_R8G8B8A8_UNORM ||
           f == VK_FORMAT_R8G8B8A8_SRGB;
}

uint32_t capture_format(VkFormat f) {
    if (f == VK_FORMAT_B8G8R8A8_UNORM || f == VK_FORMAT_B8G8R8A8_SRGB) return kCaptureBGRA;
    if (f == VK_FORMAT_R8G8B8A8_UNORM || f == VK_FORMAT_R8G8B8A8_SRGB) return kCaptureRGBA;
    return 0;
}

uint32_t find_memory_type(uint32_t bits, VkMemoryPropertyFlags want) {
    if (!g_instance.GetPhysicalDeviceMemoryProperties || g_device.physical == VK_NULL_HANDLE) return UINT32_MAX;
    VkPhysicalDeviceMemoryProperties props{};
    g_instance.GetPhysicalDeviceMemoryProperties(g_device.physical, &props);
    for (uint32_t i = 0; i < props.memoryTypeCount; ++i) {
        if ((bits & (1u << i)) && (props.memoryTypes[i].propertyFlags & want) == want) return i;
    }
    return UINT32_MAX;
}

bool create_host_buffer(VkDeviceSize bytes, VkBufferUsageFlags usage,
                        VkBuffer* outBuffer, VkDeviceMemory* outMemory, void** outMapped) {
    VkBufferCreateInfo bci{VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO};
    bci.size = std::max<VkDeviceSize>(bytes, 256);
    bci.usage = usage;
    bci.sharingMode = VK_SHARING_MODE_EXCLUSIVE;
    if (g_device.CreateBuffer(g_device.device, &bci, nullptr, outBuffer) != VK_SUCCESS) return false;

    VkMemoryRequirements req{};
    g_device.GetBufferMemoryRequirements(g_device.device, *outBuffer, &req);
    uint32_t type = find_memory_type(req.memoryTypeBits,
        VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT);
    if (type == UINT32_MAX) return false;

    VkMemoryAllocateInfo mai{VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO};
    mai.allocationSize = req.size;
    mai.memoryTypeIndex = type;
    if (g_device.AllocateMemory(g_device.device, &mai, nullptr, outMemory) != VK_SUCCESS) return false;
    if (g_device.BindBufferMemory(g_device.device, *outBuffer, *outMemory, 0) != VK_SUCCESS) return false;
    if (g_device.MapMemory(g_device.device, *outMemory, 0, VK_WHOLE_SIZE, 0, outMapped) != VK_SUCCESS) return false;
    return true;
}

void destroy_frame_buffer(FrameRes& f) {
    if (f.vertexMapped && f.vertexMemory && g_device.UnmapMemory) {
        g_device.UnmapMemory(g_device.device, f.vertexMemory);
    }
    f.vertexMapped = nullptr;
    if (f.vertexBuffer && g_device.DestroyBuffer) g_device.DestroyBuffer(g_device.device, f.vertexBuffer, nullptr);
    if (f.vertexMemory && g_device.FreeMemory) g_device.FreeMemory(g_device.device, f.vertexMemory, nullptr);
    f.vertexBuffer = VK_NULL_HANDLE;
    f.vertexMemory = VK_NULL_HANDLE;
    f.vertexCapacity = 0;
    f.generation = 0;
    f.vertexCount = 0;
}

void destroy_swap_resources(SwapState& s) {
    if (g_device.device == VK_NULL_HANDLE) return;
    if (g_device.DeviceWaitIdle) g_device.DeviceWaitIdle(g_device.device);

    for (FrameRes& f : s.frames) {
        destroy_frame_buffer(f);
        if (f.done && g_device.DestroySemaphore) g_device.DestroySemaphore(g_device.device, f.done, nullptr);
        if (f.fence && g_device.DestroyFence) g_device.DestroyFence(g_device.device, f.fence, nullptr);
    }
    s.frames.clear();

    if (s.captureMapped && s.captureMemory && g_device.UnmapMemory) g_device.UnmapMemory(g_device.device, s.captureMemory);
    s.captureMapped = nullptr;
    if (s.captureBuffer && g_device.DestroyBuffer) g_device.DestroyBuffer(g_device.device, s.captureBuffer, nullptr);
    if (s.captureMemory && g_device.FreeMemory) g_device.FreeMemory(g_device.device, s.captureMemory, nullptr);
    s.captureBuffer = VK_NULL_HANDLE;
    s.captureMemory = VK_NULL_HANDLE;
    s.captureCapacity = 0;

    for (VkFramebuffer fb : s.framebuffers) if (fb && g_device.DestroyFramebuffer) g_device.DestroyFramebuffer(g_device.device, fb, nullptr);
    s.framebuffers.clear();
    for (VkImageView v : s.views) if (v && g_device.DestroyImageView) g_device.DestroyImageView(g_device.device, v, nullptr);
    s.views.clear();

    if (s.pipeline && g_device.DestroyPipeline) g_device.DestroyPipeline(g_device.device, s.pipeline, nullptr);
    if (s.pipelineLayout && g_device.DestroyPipelineLayout) g_device.DestroyPipelineLayout(g_device.device, s.pipelineLayout, nullptr);
    if (s.renderPass && g_device.DestroyRenderPass) g_device.DestroyRenderPass(g_device.device, s.renderPass, nullptr);
    s.pipeline = VK_NULL_HANDLE;
    s.pipelineLayout = VK_NULL_HANDLE;
    s.renderPass = VK_NULL_HANDLE;

    if (s.commandPool && g_device.DestroyCommandPool) g_device.DestroyCommandPool(g_device.device, s.commandPool, nullptr);
    s.commandPool = VK_NULL_HANDLE;
    s.ready = false;
}

bool refresh_images(SwapState& s) {
    uint32_t count = 0;
    VkResult r = g_device.GetSwapchainImagesKHR(g_device.device, s.swapchain, &count, nullptr);
    if (r != VK_SUCCESS || count == 0) return false;
    s.images.resize(count);
    r = g_device.GetSwapchainImagesKHR(g_device.device, s.swapchain, &count, s.images.data());
    if (r != VK_SUCCESS && r != VK_INCOMPLETE) return false;
    s.images.resize(count);
    return !s.images.empty();
}

std::vector<uint32_t> read_spv(const std::string& path) {
    std::ifstream f(path, std::ios::binary | std::ios::ate);
    if (!f) return {};
    std::streamsize n = f.tellg();
    if (n <= 0 || (n % 4) != 0 || n > 4 * 1024 * 1024) return {};
    f.seekg(0);
    std::vector<uint32_t> out(static_cast<size_t>(n) / 4);
    if (!f.read(reinterpret_cast<char*>(out.data()), n)) return {};
    return out;
}

bool create_pipeline(SwapState& s) {
    VkAttachmentDescription attachment{};
    attachment.format = s.format;
    attachment.samples = VK_SAMPLE_COUNT_1_BIT;
    attachment.loadOp = VK_ATTACHMENT_LOAD_OP_LOAD;
    attachment.storeOp = VK_ATTACHMENT_STORE_OP_STORE;
    attachment.stencilLoadOp = VK_ATTACHMENT_LOAD_OP_DONT_CARE;
    attachment.stencilStoreOp = VK_ATTACHMENT_STORE_OP_DONT_CARE;
    attachment.initialLayout = VK_IMAGE_LAYOUT_PRESENT_SRC_KHR;
    attachment.finalLayout = VK_IMAGE_LAYOUT_PRESENT_SRC_KHR;

    VkAttachmentReference colorRef{};
    colorRef.attachment = 0;
    colorRef.layout = VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL;

    VkSubpassDescription subpass{};
    subpass.pipelineBindPoint = VK_PIPELINE_BIND_POINT_GRAPHICS;
    subpass.colorAttachmentCount = 1;
    subpass.pColorAttachments = &colorRef;

    VkSubpassDependency deps[2]{};
    deps[0].srcSubpass = VK_SUBPASS_EXTERNAL;
    deps[0].dstSubpass = 0;
    deps[0].srcStageMask = VK_PIPELINE_STAGE_ALL_COMMANDS_BIT;
    deps[0].dstStageMask = VK_PIPELINE_STAGE_COLOR_ATTACHMENT_OUTPUT_BIT;
    deps[0].srcAccessMask = 0;
    deps[0].dstAccessMask = VK_ACCESS_COLOR_ATTACHMENT_READ_BIT | VK_ACCESS_COLOR_ATTACHMENT_WRITE_BIT;
    deps[1].srcSubpass = 0;
    deps[1].dstSubpass = VK_SUBPASS_EXTERNAL;
    deps[1].srcStageMask = VK_PIPELINE_STAGE_COLOR_ATTACHMENT_OUTPUT_BIT;
    deps[1].dstStageMask = VK_PIPELINE_STAGE_BOTTOM_OF_PIPE_BIT;
    deps[1].srcAccessMask = VK_ACCESS_COLOR_ATTACHMENT_WRITE_BIT;
    deps[1].dstAccessMask = 0;

    VkRenderPassCreateInfo rpci{VK_STRUCTURE_TYPE_RENDER_PASS_CREATE_INFO};
    rpci.attachmentCount = 1;
    rpci.pAttachments = &attachment;
    rpci.subpassCount = 1;
    rpci.pSubpasses = &subpass;
    rpci.dependencyCount = 2;
    rpci.pDependencies = deps;
    if (g_device.CreateRenderPass(g_device.device, &rpci, nullptr, &s.renderPass) != VK_SUCCESS) return false;

    s.views.resize(s.images.size(), VK_NULL_HANDLE);
    s.framebuffers.resize(s.images.size(), VK_NULL_HANDLE);
    for (size_t i = 0; i < s.images.size(); ++i) {
        VkImageViewCreateInfo iv{VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO};
        iv.image = s.images[i];
        iv.viewType = VK_IMAGE_VIEW_TYPE_2D;
        iv.format = s.format;
        iv.components = {VK_COMPONENT_SWIZZLE_IDENTITY, VK_COMPONENT_SWIZZLE_IDENTITY,
                         VK_COMPONENT_SWIZZLE_IDENTITY, VK_COMPONENT_SWIZZLE_IDENTITY};
        iv.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
        iv.subresourceRange.baseMipLevel = 0;
        iv.subresourceRange.levelCount = 1;
        iv.subresourceRange.baseArrayLayer = 0;
        iv.subresourceRange.layerCount = 1;
        if (g_device.CreateImageView(g_device.device, &iv, nullptr, &s.views[i]) != VK_SUCCESS) return false;

        VkFramebufferCreateInfo fb{VK_STRUCTURE_TYPE_FRAMEBUFFER_CREATE_INFO};
        fb.renderPass = s.renderPass;
        fb.attachmentCount = 1;
        fb.pAttachments = &s.views[i];
        fb.width = s.extent.width;
        fb.height = s.extent.height;
        fb.layers = 1;
        if (g_device.CreateFramebuffer(g_device.device, &fb, nullptr, &s.framebuffers[i]) != VK_SUCCESS) return false;
    }

    std::string dir = hud_dir();
    auto vertCode = read_spv(dir + "/overlay.vert.spv");
    auto fragCode = read_spv(dir + "/overlay.frag.spv");
    if (vertCode.empty() || fragCode.empty()) {
        log_line("v10: shader files are missing or invalid");
        return false;
    }

    VkShaderModuleCreateInfo smci{VK_STRUCTURE_TYPE_SHADER_MODULE_CREATE_INFO};
    smci.codeSize = vertCode.size() * sizeof(uint32_t);
    smci.pCode = vertCode.data();
    VkShaderModule vert = VK_NULL_HANDLE;
    if (g_device.CreateShaderModule(g_device.device, &smci, nullptr, &vert) != VK_SUCCESS) return false;
    smci.codeSize = fragCode.size() * sizeof(uint32_t);
    smci.pCode = fragCode.data();
    VkShaderModule frag = VK_NULL_HANDLE;
    if (g_device.CreateShaderModule(g_device.device, &smci, nullptr, &frag) != VK_SUCCESS) {
        g_device.DestroyShaderModule(g_device.device, vert, nullptr);
        return false;
    }

    VkPipelineShaderStageCreateInfo stages[2]{};
    stages[0].sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    stages[0].stage = VK_SHADER_STAGE_VERTEX_BIT;
    stages[0].module = vert;
    stages[0].pName = "main";
    stages[1].sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    stages[1].stage = VK_SHADER_STAGE_FRAGMENT_BIT;
    stages[1].module = frag;
    stages[1].pName = "main";

    VkVertexInputBindingDescription binding{};
    binding.binding = 0;
    binding.stride = sizeof(Vertex);
    binding.inputRate = VK_VERTEX_INPUT_RATE_VERTEX;
    VkVertexInputAttributeDescription attrs[2]{};
    attrs[0].location = 0;
    attrs[0].binding = 0;
    attrs[0].format = VK_FORMAT_R32G32_SFLOAT;
    attrs[0].offset = 0;
    attrs[1].location = 1;
    attrs[1].binding = 0;
    attrs[1].format = VK_FORMAT_R32G32B32A32_SFLOAT;
    attrs[1].offset = 8;

    VkPipelineVertexInputStateCreateInfo vi{VK_STRUCTURE_TYPE_PIPELINE_VERTEX_INPUT_STATE_CREATE_INFO};
    vi.vertexBindingDescriptionCount = 1;
    vi.pVertexBindingDescriptions = &binding;
    vi.vertexAttributeDescriptionCount = 2;
    vi.pVertexAttributeDescriptions = attrs;

    VkPipelineInputAssemblyStateCreateInfo ia{VK_STRUCTURE_TYPE_PIPELINE_INPUT_ASSEMBLY_STATE_CREATE_INFO};
    ia.topology = VK_PRIMITIVE_TOPOLOGY_TRIANGLE_LIST;

    VkViewport viewport{};
    viewport.x = 0;
    viewport.y = 0;
    viewport.width = static_cast<float>(s.extent.width);
    viewport.height = static_cast<float>(s.extent.height);
    viewport.minDepth = 0;
    viewport.maxDepth = 1;
    VkRect2D scissor{{0, 0}, s.extent};
    VkPipelineViewportStateCreateInfo vp{VK_STRUCTURE_TYPE_PIPELINE_VIEWPORT_STATE_CREATE_INFO};
    vp.viewportCount = 1;
    vp.pViewports = &viewport;
    vp.scissorCount = 1;
    vp.pScissors = &scissor;

    VkPipelineRasterizationStateCreateInfo rs{VK_STRUCTURE_TYPE_PIPELINE_RASTERIZATION_STATE_CREATE_INFO};
    rs.polygonMode = VK_POLYGON_MODE_FILL;
    rs.cullMode = VK_CULL_MODE_NONE;
    rs.frontFace = VK_FRONT_FACE_COUNTER_CLOCKWISE;
    rs.lineWidth = 1.0f;

    VkPipelineMultisampleStateCreateInfo ms{VK_STRUCTURE_TYPE_PIPELINE_MULTISAMPLE_STATE_CREATE_INFO};
    ms.rasterizationSamples = VK_SAMPLE_COUNT_1_BIT;

    VkPipelineColorBlendAttachmentState blend{};
    blend.blendEnable = VK_TRUE;
    blend.srcColorBlendFactor = VK_BLEND_FACTOR_SRC_ALPHA;
    blend.dstColorBlendFactor = VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA;
    blend.colorBlendOp = VK_BLEND_OP_ADD;
    blend.srcAlphaBlendFactor = VK_BLEND_FACTOR_ONE;
    blend.dstAlphaBlendFactor = VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA;
    blend.alphaBlendOp = VK_BLEND_OP_ADD;
    blend.colorWriteMask = VK_COLOR_COMPONENT_R_BIT | VK_COLOR_COMPONENT_G_BIT |
                           VK_COLOR_COMPONENT_B_BIT | VK_COLOR_COMPONENT_A_BIT;
    VkPipelineColorBlendStateCreateInfo cb{VK_STRUCTURE_TYPE_PIPELINE_COLOR_BLEND_STATE_CREATE_INFO};
    cb.attachmentCount = 1;
    cb.pAttachments = &blend;

    VkPipelineLayoutCreateInfo plci{VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO};
    if (g_device.CreatePipelineLayout(g_device.device, &plci, nullptr, &s.pipelineLayout) != VK_SUCCESS) {
        g_device.DestroyShaderModule(g_device.device, frag, nullptr);
        g_device.DestroyShaderModule(g_device.device, vert, nullptr);
        return false;
    }

    VkGraphicsPipelineCreateInfo gp{VK_STRUCTURE_TYPE_GRAPHICS_PIPELINE_CREATE_INFO};
    gp.stageCount = 2;
    gp.pStages = stages;
    gp.pVertexInputState = &vi;
    gp.pInputAssemblyState = &ia;
    gp.pViewportState = &vp;
    gp.pRasterizationState = &rs;
    gp.pMultisampleState = &ms;
    gp.pColorBlendState = &cb;
    gp.layout = s.pipelineLayout;
    gp.renderPass = s.renderPass;
    gp.subpass = 0;
    VkResult r = g_device.CreateGraphicsPipelines(g_device.device, VK_NULL_HANDLE, 1, &gp, nullptr, &s.pipeline);

    g_device.DestroyShaderModule(g_device.device, frag, nullptr);
    g_device.DestroyShaderModule(g_device.device, vert, nullptr);
    return r == VK_SUCCESS;
}

bool prepare_swap(SwapState& s, uint32_t queueFamily) {
    if (s.ready) return s.queueFamily == queueFamily;
    if (!s.colorAttachment || !s.transferSrc || !supported_format(s.format)) {
        log_line("v10: swapchain lacks required color-attachment/transfer-src capability or format");
        return false;
    }
    if (s.images.empty() && !refresh_images(s)) return false;
    if (!create_pipeline(s)) {
        log_line("v10: pipeline creation failed");
        destroy_swap_resources(s);
        return false;
    }

    VkCommandPoolCreateInfo pci{VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO};
    pci.flags = VK_COMMAND_POOL_CREATE_RESET_COMMAND_BUFFER_BIT;
    pci.queueFamilyIndex = queueFamily;
    if (g_device.CreateCommandPool(g_device.device, &pci, nullptr, &s.commandPool) != VK_SUCCESS) {
        destroy_swap_resources(s);
        return false;
    }

    s.frames.resize(s.images.size());
    std::vector<VkCommandBuffer> cbs(s.frames.size());
    VkCommandBufferAllocateInfo cai{VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO};
    cai.commandPool = s.commandPool;
    cai.level = VK_COMMAND_BUFFER_LEVEL_PRIMARY;
    cai.commandBufferCount = static_cast<uint32_t>(cbs.size());
    if (g_device.AllocateCommandBuffers(g_device.device, &cai, cbs.data()) != VK_SUCCESS) {
        destroy_swap_resources(s);
        return false;
    }

    VkSemaphoreCreateInfo sci{VK_STRUCTURE_TYPE_SEMAPHORE_CREATE_INFO};
    VkFenceCreateInfo fci{VK_STRUCTURE_TYPE_FENCE_CREATE_INFO};
    fci.flags = VK_FENCE_CREATE_SIGNALED_BIT;
    for (size_t i = 0; i < s.frames.size(); ++i) {
        s.frames[i].commandBuffer = cbs[i];
        if (g_device.CreateSemaphore(g_device.device, &sci, nullptr, &s.frames[i].done) != VK_SUCCESS ||
            g_device.CreateFence(g_device.device, &fci, nullptr, &s.frames[i].fence) != VK_SUCCESS) {
            destroy_swap_resources(s);
            return false;
        }
    }

    VkDeviceSize captureBytes = static_cast<VkDeviceSize>(s.extent.width) * s.extent.height * 4u;
    if (!create_host_buffer(captureBytes, VK_BUFFER_USAGE_TRANSFER_DST_BIT,
                            &s.captureBuffer, &s.captureMemory, &s.captureMapped)) {
        log_line("v10: capture buffer allocation failed");
        destroy_swap_resources(s);
        return false;
    }
    s.captureCapacity = captureBytes;
    s.queueFamily = queueFamily;
    s.ready = true;

    char msg[256];
    std::snprintf(msg, sizeof(msg), "v10 ready: extent=%ux%u images=%zu format=%d qf=%u",
                  s.extent.width, s.extent.height, s.images.size(), static_cast<int>(s.format), queueFamily);
    log_line(msg);
    return true;
}

bool refresh_scene_locked() {
    std::string path = runtime_dir() + "/overlay.vertices";
    struct stat st{};
    if (::stat(path.c_str(), &st) != 0) return false;
    long ns = 0;
#if defined(__linux__)
    ns = st.st_mtim.tv_nsec;
#endif
    if (g_scene.fileSize == st.st_size && g_scene.mtimeSec == st.st_mtime && g_scene.mtimeNSec == ns) return true;
    if (st.st_size < static_cast<off_t>(kHUDHeaderBytes) || static_cast<size_t>(st.st_size) > kMaxTransportBytes) return false;

    std::ifstream f(path, std::ios::binary);
    if (!f) return false;
    std::vector<uint8_t> data(static_cast<size_t>(st.st_size));
    if (!f.read(reinterpret_cast<char*>(data.data()), static_cast<std::streamsize>(data.size()))) return false;

    if (u32le(data.data()) != kHUDMagic || u32le(data.data() + 4) != kHUDVersion) return false;
    uint64_t generation = u64le(data.data() + 8);
    uint32_t visible = u32le(data.data() + 16);
    uint32_t count = u32le(data.data() + 20);
    uint32_t canvasW = u32le(data.data() + 24);
    uint32_t canvasH = u32le(data.data() + 28);
    if (canvasW == 0 || canvasH == 0 || count > 1000000u) return false;
    size_t expected = kHUDHeaderBytes + static_cast<size_t>(count) * sizeof(Vertex);
    if (expected != data.size()) return false;

    std::vector<Vertex> vertices(count);
    if (count) std::memcpy(vertices.data(), data.data() + kHUDHeaderBytes, count * sizeof(Vertex));
    g_scene.generation = generation;
    g_scene.visible = visible != 0;
    g_scene.vertices.swap(vertices);
    g_scene.fileSize = st.st_size;
    g_scene.mtimeSec = st.st_mtime;
    g_scene.mtimeNSec = ns;
    return true;
}

uint64_t read_capture_request() {
    std::ifstream f(runtime_dir() + "/capture.request");
    if (!f) return 0;
    unsigned long long v = 0;
    f >> v;
    return static_cast<uint64_t>(v);
}

bool publish_capture(SwapState& s) {
    if (!s.captureInFlightSeq || !s.captureMapped || s.captureImageIndex >= s.frames.size()) return false;
    FrameRes& f = s.frames[s.captureImageIndex];
    if (g_device.GetFenceStatus(g_device.device, f.fence) != VK_SUCCESS) return false;

    size_t payload = static_cast<size_t>(s.extent.width) * s.extent.height * 4u;
    std::vector<uint8_t> out(kCaptureHeaderBytes + payload);
    put_u32le(out.data(), kCaptureMagic);
    put_u32le(out.data() + 4, kCaptureVersion);
    put_u64le(out.data() + 8, s.captureInFlightSeq);
    put_u32le(out.data() + 16, s.extent.width);
    put_u32le(out.data() + 20, s.extent.height);
    put_u32le(out.data() + 24, capture_format(s.format));
    put_u32le(out.data() + 28, static_cast<uint32_t>(payload));
    std::memcpy(out.data() + kCaptureHeaderBytes, s.captureMapped, payload);
    if (!atomic_write(runtime_dir() + "/capture.frame", out.data(), out.size())) {
        log_line("v10: failed to publish pre-HUD capture frame");
        return false;
    }
    s.captureLastCompleted = s.captureInFlightSeq;
    s.captureInFlightSeq = 0;
    s.captureImageIndex = UINT32_MAX;
    return true;
}

void ready_heartbeat(const SwapState& s) {
    auto now = std::chrono::steady_clock::now();
    if (g_lastReadyWrite.time_since_epoch().count() != 0 && now - g_lastReadyWrite < std::chrono::milliseconds(750)) return;
    g_lastReadyWrite = now;
    std::ostringstream ss;
    ss << "pid=" << static_cast<int>(getpid())
       << " hud=1 capture=1 width=" << s.extent.width
       << " height=" << s.extent.height
       << " format=" << static_cast<int>(s.format) << "\n";
    atomic_write_text(runtime_dir() + "/renderer.ready", ss.str());
}

bool ensure_vertex_buffer(FrameRes& f, VkDeviceSize bytes) {
    if (bytes == 0) return true;
    if (f.vertexBuffer && f.vertexCapacity >= bytes) return true;
    destroy_frame_buffer(f);
    VkDeviceSize cap = 4096;
    while (cap < bytes && cap < static_cast<VkDeviceSize>(kMaxTransportBytes)) cap *= 2;
    if (cap < bytes) return false;
    if (!create_host_buffer(cap, VK_BUFFER_USAGE_VERTEX_BUFFER_BIT, &f.vertexBuffer, &f.vertexMemory, &f.vertexMapped)) return false;
    f.vertexCapacity = cap;
    return true;
}

bool record_and_submit(VkQueue queue, SwapState& s, uint32_t imageIndex,
                       const VkPresentInfoKHR* present, uint64_t requestSeq,
                       VkSemaphore* outWait) {
    if (imageIndex >= s.frames.size() || imageIndex >= s.framebuffers.size()) return false;
    FrameRes& fr = s.frames[imageIndex];

    if (fr.inFlight) {
        if (g_device.WaitForFences(g_device.device, 1, &fr.fence, VK_TRUE, UINT64_MAX) != VK_SUCCESS) return false;
        fr.inFlight = false;
        if (s.captureInFlightSeq && s.captureImageIndex == imageIndex) publish_capture(s);
    }

    bool render = g_scene.visible && !g_scene.vertices.empty();
    bool capture = requestSeq > s.captureLastCompleted && s.captureInFlightSeq == 0;
    if (!render && !capture) return false;

    if (render && fr.generation != g_scene.generation) {
        VkDeviceSize bytes = static_cast<VkDeviceSize>(g_scene.vertices.size()) * sizeof(Vertex);
        if (!ensure_vertex_buffer(fr, bytes)) {
            log_line("v10: vertex buffer allocation failed");
            return false;
        }
        if (bytes) std::memcpy(fr.vertexMapped, g_scene.vertices.data(), static_cast<size_t>(bytes));
        fr.vertexCount = static_cast<uint32_t>(g_scene.vertices.size());
        fr.generation = g_scene.generation;
    } else if (!render) {
        fr.vertexCount = 0;
        fr.generation = g_scene.generation;
    }

    if (g_device.ResetFences(g_device.device, 1, &fr.fence) != VK_SUCCESS) return false;
    if (g_device.ResetCommandBuffer(fr.commandBuffer, 0) != VK_SUCCESS) return false;

    VkCommandBufferBeginInfo bi{VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO};
    bi.flags = VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT;
    if (g_device.BeginCommandBuffer(fr.commandBuffer, &bi) != VK_SUCCESS) return false;

    if (capture) {
        VkImageMemoryBarrier toCopy{VK_STRUCTURE_TYPE_IMAGE_MEMORY_BARRIER};
        toCopy.srcAccessMask = 0;
        toCopy.dstAccessMask = VK_ACCESS_TRANSFER_READ_BIT;
        toCopy.oldLayout = VK_IMAGE_LAYOUT_PRESENT_SRC_KHR;
        toCopy.newLayout = VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL;
        toCopy.srcQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED;
        toCopy.dstQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED;
        toCopy.image = s.images[imageIndex];
        toCopy.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
        toCopy.subresourceRange.levelCount = 1;
        toCopy.subresourceRange.layerCount = 1;
        g_device.CmdPipelineBarrier(fr.commandBuffer,
            VK_PIPELINE_STAGE_ALL_COMMANDS_BIT, VK_PIPELINE_STAGE_TRANSFER_BIT,
            0, 0, nullptr, 0, nullptr, 1, &toCopy);

        VkBufferImageCopy copy{};
        copy.imageSubresource.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
        copy.imageSubresource.layerCount = 1;
        copy.imageExtent = {s.extent.width, s.extent.height, 1};
        g_device.CmdCopyImageToBuffer(fr.commandBuffer, s.images[imageIndex],
            VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL, s.captureBuffer, 1, &copy);

        VkImageMemoryBarrier back = toCopy;
        back.srcAccessMask = VK_ACCESS_TRANSFER_READ_BIT;
        back.dstAccessMask = 0;
        back.oldLayout = VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL;
        back.newLayout = VK_IMAGE_LAYOUT_PRESENT_SRC_KHR;
        g_device.CmdPipelineBarrier(fr.commandBuffer,
            VK_PIPELINE_STAGE_TRANSFER_BIT, VK_PIPELINE_STAGE_BOTTOM_OF_PIPE_BIT,
            0, 0, nullptr, 0, nullptr, 1, &back);
    }

    if (render && fr.vertexCount > 0) {
        VkRenderPassBeginInfo rp{VK_STRUCTURE_TYPE_RENDER_PASS_BEGIN_INFO};
        rp.renderPass = s.renderPass;
        rp.framebuffer = s.framebuffers[imageIndex];
        rp.renderArea.offset = {0, 0};
        rp.renderArea.extent = s.extent;
        g_device.CmdBeginRenderPass(fr.commandBuffer, &rp, VK_SUBPASS_CONTENTS_INLINE);
        g_device.CmdBindPipeline(fr.commandBuffer, VK_PIPELINE_BIND_POINT_GRAPHICS, s.pipeline);
        VkDeviceSize offset = 0;
        g_device.CmdBindVertexBuffers(fr.commandBuffer, 0, 1, &fr.vertexBuffer, &offset);
        g_device.CmdDraw(fr.commandBuffer, fr.vertexCount, 1, 0, 0);
        g_device.CmdEndRenderPass(fr.commandBuffer);
    }

    if (g_device.EndCommandBuffer(fr.commandBuffer) != VK_SUCCESS) return false;

    std::vector<VkPipelineStageFlags> waitStages(present->waitSemaphoreCount, VK_PIPELINE_STAGE_ALL_COMMANDS_BIT);
    VkSubmitInfo si{VK_STRUCTURE_TYPE_SUBMIT_INFO};
    si.waitSemaphoreCount = present->waitSemaphoreCount;
    si.pWaitSemaphores = present->pWaitSemaphores;
    si.pWaitDstStageMask = waitStages.empty() ? nullptr : waitStages.data();
    si.commandBufferCount = 1;
    si.pCommandBuffers = &fr.commandBuffer;
    si.signalSemaphoreCount = 1;
    si.pSignalSemaphores = &fr.done;
    if (g_device.QueueSubmit(queue, 1, &si, fr.fence) != VK_SUCCESS) return false;
    fr.inFlight = true;
    if (capture) {
        s.captureInFlightSeq = requestSeq;
        s.captureImageIndex = imageIndex;
    }
    *outWait = fr.done;
    return true;
}

VkResult VKAPI_CALL layerCreateInstance(const VkInstanceCreateInfo* ci,
                                        const VkAllocationCallbacks* allocator,
                                        VkInstance* out) {
    VkLayerInstanceCreateInfo* chain = instance_chain(ci, VK_LAYER_LINK_INFO);
    if (!chain || !chain->u.pLayerInfo) return VK_ERROR_INITIALIZATION_FAILED;
    PFN_vkGetInstanceProcAddr nextGIPA = chain->u.pLayerInfo->pfnNextGetInstanceProcAddr;
    PFN_GetPhysicalDeviceProcAddr nextGPDPA = chain->u.pLayerInfo->pfnNextGetPhysicalDeviceProcAddr;
    chain->u.pLayerInfo = chain->u.pLayerInfo->pNext;
    auto nextCreate = reinterpret_cast<PFN_vkCreateInstance>(nextGIPA(VK_NULL_HANDLE, "vkCreateInstance"));
    if (!nextCreate) return VK_ERROR_INITIALIZATION_FAILED;
    VkResult r = nextCreate(ci, allocator, out);
    if (r != VK_SUCCESS) return r;

    std::lock_guard<std::mutex> lock(g_mu);
    g_instance.instance = *out;
    g_instance.nextGIPA = nextGIPA;
    g_instance.nextGPDPA = nextGPDPA;
    g_instance.DestroyInstance = reinterpret_cast<PFN_vkDestroyInstance>(nextGIPA(*out, "vkDestroyInstance"));
    g_instance.CreateDevice = reinterpret_cast<PFN_vkCreateDevice>(nextGIPA(*out, "vkCreateDevice"));
    g_instance.GetPhysicalDeviceMemoryProperties = reinterpret_cast<PFN_vkGetPhysicalDeviceMemoryProperties>(
        nextGIPA(*out, "vkGetPhysicalDeviceMemoryProperties"));
    log_line("VK_LAYER_JACOB_hud v10 active");
    return r;
}

void VKAPI_CALL layerDestroyInstance(VkInstance instance, const VkAllocationCallbacks* allocator) {
    PFN_vkDestroyInstance next = nullptr;
    {
        std::lock_guard<std::mutex> lock(g_mu);
        next = g_instance.DestroyInstance;
        g_instance = {};
    }
    if (next) next(instance, allocator);
}

VkResult VKAPI_CALL layerCreateDevice(VkPhysicalDevice physical,
                                      const VkDeviceCreateInfo* ci,
                                      const VkAllocationCallbacks* allocator,
                                      VkDevice* out) {
    VkLayerDeviceCreateInfo* chain = device_chain(ci, VK_LAYER_LINK_INFO);
    if (!chain || !chain->u.pLayerInfo) return VK_ERROR_INITIALIZATION_FAILED;
    PFN_vkGetInstanceProcAddr nextGIPA = chain->u.pLayerInfo->pfnNextGetInstanceProcAddr;
    PFN_vkGetDeviceProcAddr nextGDPA = chain->u.pLayerInfo->pfnNextGetDeviceProcAddr;
    chain->u.pLayerInfo = chain->u.pLayerInfo->pNext;

    PFN_vkCreateDevice nextCreate = nullptr;
    {
        std::lock_guard<std::mutex> lock(g_mu);
        if (g_instance.instance != VK_NULL_HANDLE) nextCreate = reinterpret_cast<PFN_vkCreateDevice>(nextGIPA(g_instance.instance, "vkCreateDevice"));
    }
    if (!nextCreate) return VK_ERROR_INITIALIZATION_FAILED;
    VkResult r = nextCreate(physical, ci, allocator, out);
    if (r != VK_SUCCESS) return r;

    std::lock_guard<std::mutex> lock(g_mu);
    g_device = {};
    g_device.physical = physical;
    g_device.device = *out;
    g_device.nextGDPA = nextGDPA;
#define LOAD_DEV(name) g_device.name = load_dev<PFN_vk##name>("vk" #name)
    LOAD_DEV(DestroyDevice);
    LOAD_DEV(GetDeviceQueue);
    LOAD_DEV(GetDeviceQueue2);
    LOAD_DEV(QueueSubmit);
    LOAD_DEV(DeviceWaitIdle);
    LOAD_DEV(CreateSwapchainKHR);
    LOAD_DEV(DestroySwapchainKHR);
    LOAD_DEV(GetSwapchainImagesKHR);
    LOAD_DEV(QueuePresentKHR);
    LOAD_DEV(CreateCommandPool);
    LOAD_DEV(DestroyCommandPool);
    LOAD_DEV(AllocateCommandBuffers);
    LOAD_DEV(FreeCommandBuffers);
    LOAD_DEV(ResetCommandBuffer);
    LOAD_DEV(BeginCommandBuffer);
    LOAD_DEV(EndCommandBuffer);
    LOAD_DEV(CreateSemaphore);
    LOAD_DEV(DestroySemaphore);
    LOAD_DEV(CreateFence);
    LOAD_DEV(DestroyFence);
    LOAD_DEV(ResetFences);
    LOAD_DEV(WaitForFences);
    LOAD_DEV(GetFenceStatus);
    LOAD_DEV(CreateBuffer);
    LOAD_DEV(DestroyBuffer);
    LOAD_DEV(GetBufferMemoryRequirements);
    LOAD_DEV(AllocateMemory);
    LOAD_DEV(FreeMemory);
    LOAD_DEV(BindBufferMemory);
    LOAD_DEV(MapMemory);
    LOAD_DEV(UnmapMemory);
    LOAD_DEV(CreateImageView);
    LOAD_DEV(DestroyImageView);
    LOAD_DEV(CreateRenderPass);
    LOAD_DEV(DestroyRenderPass);
    LOAD_DEV(CreateFramebuffer);
    LOAD_DEV(DestroyFramebuffer);
    LOAD_DEV(CreateShaderModule);
    LOAD_DEV(DestroyShaderModule);
    LOAD_DEV(CreatePipelineLayout);
    LOAD_DEV(DestroyPipelineLayout);
    LOAD_DEV(CreateGraphicsPipelines);
    LOAD_DEV(DestroyPipeline);
    LOAD_DEV(CmdPipelineBarrier);
    LOAD_DEV(CmdCopyImageToBuffer);
    LOAD_DEV(CmdBeginRenderPass);
    LOAD_DEV(CmdEndRenderPass);
    LOAD_DEV(CmdBindPipeline);
    LOAD_DEV(CmdBindVertexBuffers);
    LOAD_DEV(CmdDraw);
#undef LOAD_DEV
    log_line("v10: Vulkan device chained");
    return r;
}

void VKAPI_CALL layerDestroyDevice(VkDevice device, const VkAllocationCallbacks* allocator) {
    PFN_vkDestroyDevice next = nullptr;
    {
        std::lock_guard<std::mutex> lock(g_mu);
        if (g_device.DeviceWaitIdle) g_device.DeviceWaitIdle(device);
        for (auto& it : g_swaps) destroy_swap_resources(*it.second);
        g_swaps.clear();
        g_queueFamilies.clear();
        next = g_device.DestroyDevice;
        g_device = {};
    }
    if (next) next(device, allocator);
}

void VKAPI_CALL layerGetDeviceQueue(VkDevice device, uint32_t family, uint32_t index, VkQueue* out) {
    PFN_vkGetDeviceQueue next = nullptr;
    { std::lock_guard<std::mutex> lock(g_mu); next = g_device.GetDeviceQueue; }
    next(device, family, index, out);
    if (out && *out) { std::lock_guard<std::mutex> lock(g_mu); g_queueFamilies[key(*out)] = family; }
}

void VKAPI_CALL layerGetDeviceQueue2(VkDevice device, const VkDeviceQueueInfo2* info, VkQueue* out) {
    PFN_vkGetDeviceQueue2 next = nullptr;
    { std::lock_guard<std::mutex> lock(g_mu); next = g_device.GetDeviceQueue2; }
    next(device, info, out);
    if (info && out && *out) { std::lock_guard<std::mutex> lock(g_mu); g_queueFamilies[key(*out)] = info->queueFamilyIndex; }
}

VkResult VKAPI_CALL layerCreateSwapchainKHR(VkDevice device,
                                             const VkSwapchainCreateInfoKHR* ci,
                                             const VkAllocationCallbacks* allocator,
                                             VkSwapchainKHR* out) {
    PFN_vkCreateSwapchainKHR next = nullptr;
    { std::lock_guard<std::mutex> lock(g_mu); next = g_device.CreateSwapchainKHR; }
    if (!next) return VK_ERROR_INITIALIZATION_FAILED;

    VkImageUsageFlags required = VK_IMAGE_USAGE_COLOR_ATTACHMENT_BIT | VK_IMAGE_USAGE_TRANSFER_SRC_BIT;
    VkSwapchainCreateInfoKHR modified = *ci;
    modified.imageUsage |= required;
    VkResult r = next(device, &modified, allocator, out);
    VkImageUsageFlags actualUsage = modified.imageUsage;
    if (r != VK_SUCCESS) {
        r = next(device, ci, allocator, out);
        actualUsage = ci->imageUsage;
    }
    if (r != VK_SUCCESS) return r;

    auto state = std::make_unique<SwapState>();
    state->swapchain = *out;
    state->format = ci->imageFormat;
    state->extent = ci->imageExtent;
    state->colorAttachment = (actualUsage & VK_IMAGE_USAGE_COLOR_ATTACHMENT_BIT) != 0;
    state->transferSrc = (actualUsage & VK_IMAGE_USAGE_TRANSFER_SRC_BIT) != 0;
    {
        std::lock_guard<std::mutex> lock(g_mu);
        g_swaps[key(*out)] = std::move(state);
    }
    log_line("v10: swapchain tracked");
    return r;
}

void VKAPI_CALL layerDestroySwapchainKHR(VkDevice device, VkSwapchainKHR swapchain,
                                          const VkAllocationCallbacks* allocator) {
    PFN_vkDestroySwapchainKHR next = nullptr;
    {
        std::lock_guard<std::mutex> lock(g_mu);
        auto it = g_swaps.find(key(swapchain));
        if (it != g_swaps.end()) {
            destroy_swap_resources(*it->second);
            g_swaps.erase(it);
        }
        next = g_device.DestroySwapchainKHR;
    }
    if (next) next(device, swapchain, allocator);
}

VkResult VKAPI_CALL layerGetSwapchainImagesKHR(VkDevice device, VkSwapchainKHR swapchain,
                                                uint32_t* count, VkImage* images) {
    PFN_vkGetSwapchainImagesKHR next = nullptr;
    { std::lock_guard<std::mutex> lock(g_mu); next = g_device.GetSwapchainImagesKHR; }
    if (!next) return VK_ERROR_INITIALIZATION_FAILED;
    VkResult r = next(device, swapchain, count, images);
    if (images && count && (r == VK_SUCCESS || r == VK_INCOMPLETE)) {
        std::lock_guard<std::mutex> lock(g_mu);
        auto it = g_swaps.find(key(swapchain));
        if (it != g_swaps.end()) it->second->images.assign(images, images + *count);
    }
    return r;
}

VkResult VKAPI_CALL layerQueuePresentKHR(VkQueue queue, const VkPresentInfoKHR* present) {
    PFN_vkQueuePresentKHR nextPresent = nullptr;
    VkSemaphore layerWait = VK_NULL_HANDLE;
    bool patched = false;

    {
        std::unique_lock<std::mutex> lock(g_mu);
        nextPresent = g_device.QueuePresentKHR;
        if (!nextPresent || !present || present->swapchainCount == 0 || !present->pSwapchains || !present->pImageIndices) {
            lock.unlock();
            return nextPresent ? nextPresent(queue, present) : VK_ERROR_INITIALIZATION_FAILED;
        }

        auto qit = g_queueFamilies.find(key(queue));
        auto sit = g_swaps.find(key(present->pSwapchains[0]));
        if (qit != g_queueFamilies.end() && sit != g_swaps.end()) {
            SwapState& s = *sit->second;
            if (prepare_swap(s, qit->second)) {
                ready_heartbeat(s);
                refresh_scene_locked();
                publish_capture(s);
                uint64_t request = read_capture_request();
                patched = record_and_submit(queue, s, present->pImageIndices[0], present, request, &layerWait);
            }
        }
    }

    if (!patched) return nextPresent(queue, present);
    VkPresentInfoKHR p = *present;
    p.waitSemaphoreCount = 1;
    p.pWaitSemaphores = &layerWait;
    VkResult r = nextPresent(queue, &p);

    {
        std::lock_guard<std::mutex> lock(g_mu);
        auto sit = g_swaps.find(key(present->pSwapchains[0]));
        if (sit != g_swaps.end()) publish_capture(*sit->second);
    }
    return r;
}

} // namespace

extern "C" {

VKAPI_ATTR PFN_vkVoidFunction VKAPI_CALL jacobGetDeviceProcAddr(VkDevice device, const char* name) {
    if (!name) return nullptr;
    if (std::strcmp(name, "vkGetDeviceProcAddr") == 0) return reinterpret_cast<PFN_vkVoidFunction>(jacobGetDeviceProcAddr);
    if (std::strcmp(name, "vkDestroyDevice") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerDestroyDevice);
    if (std::strcmp(name, "vkGetDeviceQueue") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerGetDeviceQueue);
    if (std::strcmp(name, "vkGetDeviceQueue2") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerGetDeviceQueue2);
    if (std::strcmp(name, "vkCreateSwapchainKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerCreateSwapchainKHR);
    if (std::strcmp(name, "vkDestroySwapchainKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerDestroySwapchainKHR);
    if (std::strcmp(name, "vkGetSwapchainImagesKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerGetSwapchainImagesKHR);
    if (std::strcmp(name, "vkQueuePresentKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerQueuePresentKHR);
    std::lock_guard<std::mutex> lock(g_mu);
    return g_device.nextGDPA ? g_device.nextGDPA(device, name) : nullptr;
}

VKAPI_ATTR PFN_vkVoidFunction VKAPI_CALL jacobGetInstanceProcAddr(VkInstance instance, const char* name) {
    if (!name) return nullptr;
    if (std::strcmp(name, "vkGetInstanceProcAddr") == 0) return reinterpret_cast<PFN_vkVoidFunction>(jacobGetInstanceProcAddr);
    if (std::strcmp(name, "vkGetDeviceProcAddr") == 0) return reinterpret_cast<PFN_vkVoidFunction>(jacobGetDeviceProcAddr);
    if (std::strcmp(name, "vkCreateInstance") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerCreateInstance);
    if (std::strcmp(name, "vkDestroyInstance") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerDestroyInstance);
    if (std::strcmp(name, "vkCreateDevice") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerCreateDevice);
    if (std::strcmp(name, "vkDestroyDevice") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerDestroyDevice);
    if (std::strcmp(name, "vkGetDeviceQueue") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerGetDeviceQueue);
    if (std::strcmp(name, "vkGetDeviceQueue2") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerGetDeviceQueue2);
    if (std::strcmp(name, "vkCreateSwapchainKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerCreateSwapchainKHR);
    if (std::strcmp(name, "vkDestroySwapchainKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerDestroySwapchainKHR);
    if (std::strcmp(name, "vkGetSwapchainImagesKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerGetSwapchainImagesKHR);
    if (std::strcmp(name, "vkQueuePresentKHR") == 0) return reinterpret_cast<PFN_vkVoidFunction>(layerQueuePresentKHR);
    std::lock_guard<std::mutex> lock(g_mu);
    return g_instance.nextGIPA ? g_instance.nextGIPA(instance, name) : nullptr;
}

VKAPI_ATTR PFN_vkVoidFunction VKAPI_CALL jacobGetPhysicalDeviceProcAddr(VkInstance instance, const char* name) {
    std::lock_guard<std::mutex> lock(g_mu);
    if (g_instance.nextGPDPA) return g_instance.nextGPDPA(instance, name);
    if (g_instance.nextGIPA) return g_instance.nextGIPA(instance, name);
    return nullptr;
}

JACOB_EXPORT VKAPI_ATTR VkResult VKAPI_CALL vkNegotiateLoaderLayerInterfaceVersion(VkNegotiateLayerInterface* iface) {
    if (!iface) return VK_ERROR_INITIALIZATION_FAILED;
    if (iface->loaderLayerInterfaceVersion > 2) iface->loaderLayerInterfaceVersion = 2;
    if (iface->loaderLayerInterfaceVersion < 2) return VK_ERROR_INITIALIZATION_FAILED;
    iface->pfnGetInstanceProcAddr = jacobGetInstanceProcAddr;
    iface->pfnGetDeviceProcAddr = jacobGetDeviceProcAddr;
    iface->pfnGetPhysicalDeviceProcAddr = jacobGetPhysicalDeviceProcAddr;
    return VK_SUCCESS;
}

} // extern "C"
