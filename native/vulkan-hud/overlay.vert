#version 450
layout(location = 0) in vec2 inPos;
layout(location = 1) in vec4 inColor;
layout(location = 0) out vec4 fragColor;
void main() {
    vec2 ndc = vec2(inPos.x * 2.0 - 1.0, 1.0 - inPos.y * 2.0);
    gl_Position = vec4(ndc, 0.0, 1.0);
    fragColor = inColor;
}
