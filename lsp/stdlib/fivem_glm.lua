---@meta
-- FiveM / LuaGLM — Vector, Matrix, and Quaternion type declarations.
-- These types exist as first-class types in the FiveM runtime (type 4 = vector, type 10 = matrix).
-- The Lua 5.4 type number for string is shifted from 4 to 5, and all subsequent types shift by +1.

-- =============================================================================
-- Vector Types
-- =============================================================================

---@class Vector2
---@field x number
---@field y number
local Vector2 = {}

---@class Vector3
---@field x number
---@field y number
---@field z number
local Vector3 = {}

---@class Vector4
---@field x number
---@field y number
---@field z number
---@field w number
local Vector4 = {}

-- =============================================================================
-- Quaternion Type
-- =============================================================================

---@class Quat : Vector4
local Quat = {}

-- =============================================================================
-- Matrix Types
-- =============================================================================

---@class Matrix2x2
local Matrix2x2 = {}
---@class Matrix2x3
local Matrix2x3 = {}
---@class Matrix2x4
local Matrix2x4 = {}
---@class Matrix3x2
local Matrix3x2 = {}
---@class Matrix3x3
local Matrix3x3 = {}
---@class Matrix3x4
local Matrix3x4 = {}
---@class Matrix4x2
local Matrix4x2 = {}
---@class Matrix4x3
local Matrix4x3 = {}
---@class Matrix4x4
local Matrix4x4 = {}

-- =============================================================================
-- Float Vector Constructors (global)
-- All variants: vec / vector are aliases
-- =============================================================================

---@param x? number
---@return Vector2
function vec2(x) end

---@overload fun(x: number, y: number, z: number): Vector3
---@param x? number
---@param y? number
---@return Vector3
function vec3(x, y) end

---@overload fun(x: number, y: number, z: number, w: number): Vector4
---@param x? number
---@param y? number
---@param z? number
---@return Vector4
function vec4(x, y, z) end

---@return Vector2
function vec(x) end
function vec1() end
function vector() end
function vector1() end
function vector2() end
function vector3() end
function vector4() end

-- =============================================================================
-- Integer Vector Constructors (global)
-- =============================================================================

---@return Vector2
function ivec(x) end
function ivec1() end
function ivec2() end
function ivec3() end
function ivec4() end

-- =============================================================================
-- Boolean Vector Constructors (global)
-- =============================================================================

---@return Vector2
function bvec(x) end
function bvec1() end
function bvec2() end
function bvec3() end
function bvec4() end

-- =============================================================================
-- Matrix Constructors (global)
-- =============================================================================

---@return Matrix2x2
function mat2x2() end
---@return Matrix2x2
function mat2() end

---@return Matrix2x3
function mat2x3() end
---@return Matrix2x4
function mat2x4() end
---@return Matrix3x2
function mat3x2() end

---@return Matrix3x3
function mat3x3() end
---@return Matrix3x3
function mat3() end

---@return Matrix3x4
function mat3x4() end
---@return Matrix4x2
function mat4x2() end
---@return Matrix4x3
function mat4x3() end

---@return Matrix4x4
function mat4x4() end
---@return Matrix4x4
function mat4() end

-- =============================================================================
-- Quaternion Constructors (global)
-- =============================================================================

---@param x? number
---@param y? number
---@param z? number
---@param w? number
---@return Quat
function quat(x, y, z, w) end
function qua() end

-- =============================================================================
-- Global Vector Operations (grit-lua compatibility layer)
-- These are callable as bare global functions: dot(a,b), cross(a,b), etc.
-- =============================================================================

---@param a Vector3
---@param b Vector3
---@return number
function dot(a, b) end

---@param a Vector3
---@param b Vector3
---@return Vector3
function cross(a, b) end

---@param v Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function inv(v) end

---@param v Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function norm(v) end

---@param a Quat
---@param b Quat
---@param t number
---@return Quat
function slerp(a, b, t) end

-- =============================================================================
-- Vector Instance Methods
-- All vector types (Vector2, Vector3, Vector4, Quat) share this interface.
-- Arithmetic metamethods (__add, __sub, __mul, __div, __unm, __eq, __tostring)
-- support component-wise operations and scalar multiplication/division.
-- =============================================================================

---@return Vector2|Vector3|Vector4
function Vector3:clone() end

---@param other Vector2|Vector3|Vector4
---@return number
function Vector3:dot(other) end

---@param other Vector3
---@return Vector3
function Vector3:cross(other) end

---@return number
function Vector3:length() end

---@return number
function Vector3:length2() end

---@return Vector2|Vector3|Vector4
function Vector3:normalize() end

---@return Vector2|Vector3|Vector4
function Vector3:inverse() end

---@param min number
---@param max number
---@return Vector2|Vector3|Vector4
function Vector3:clamp(min, max) end

---@param target Vector2|Vector3|Vector4
---@param t number
---@return Vector2|Vector3|Vector4
function Vector3:lerp(target, t) end

---@param other Vector3
---@return number
function Vector3:angle(other) end

---@return Vector2|Vector3|Vector4
function Vector3:normalizeAngle() end

---Resets all components to zero.
---@return Vector2|Vector3|Vector4
function Vector3:zero() end

-- =============================================================================
-- Matrix Instance Methods
-- =============================================================================

---@return Matrix3x3
function Matrix3x3:inverse() end

---@return Matrix3x3
function Matrix3x3:transpose() end

---@return number
function Matrix3x3:determinant() end

---@return Matrix3x3
function Matrix3x3:identity() end

function Matrix3x3:setIdentity() end

---@return Matrix3x3
function Matrix3x3:zero() end

-- =============================================================================
-- GLM Namespace (glm.)
-- Full GLM 0.9.9.9 binding exposed through the "glm" table.
-- Loaded via require("glm") or implicitly available in FiveM.
-- =============================================================================

glm = {}

-- Vector utilities
---@param v Vector2|Vector3|Vector4
---@return number
function glm.length(v) end

---@param a Vector2|Vector3|Vector4
---@param b Vector2|Vector3|Vector4
---@return number
function glm.distance(a, b) end

---@param a Vector2|Vector3|Vector4
---@param b Vector2|Vector3|Vector4
---@return number
function glm.dot(a, b) end

---@param a Vector3
---@param b Vector3
---@return Vector3
function glm.cross(a, b) end

---@param v Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function glm.normalize(v) end

---@param v Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function glm.inverse(v) end

---@param a Quat
---@param b Quat
---@param t number
---@return Quat
function glm.slerp(a, b, t) end

---@param v Vector2|Vector3|Vector4
---@param min number
---@param max number
---@return Vector2|Vector3|Vector4
function glm.clamp(v, min, max) end

---@param a Vector2|Vector3|Vector4
---@param b Vector2|Vector3|Vector4
---@param t number
---@return Vector2|Vector3|Vector4
function glm.lerp(a, b, t) end

---@param a Vector2|Vector3|Vector4
---@param b Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function glm.mix(a, b, t) end

---@param v Vector2|Vector3|Vector4
---@param lower Vector2|Vector3|Vector4
---@param upper Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function glm.step(v, lower, upper) end

---@param v Vector2|Vector3|Vector4
---@return number
function glm.length2(v) end

---@param degrees number
---@return number
function glm.radians(degrees) end

---@param radians number
---@return number
function glm.degrees(radians) end

-- Matrix utilities
---@param m Matrix4x4
---@return Matrix4x4
function glm.inverse(m) end

---@param m Matrix4x4
---@return Matrix4x4
function glm.transpose(m) end

---@param m Matrix4x4
---@return number
function glm.determinant(m) end

---@return Matrix4x4
function glm.identity() end

-- Matrix * vector transformation
---@param m Matrix4x4
---@param v Vector4
---@return Vector4
function glm.transform(m, v) end

-- Projection / view matrices
---@param fov number
---@param aspect number
---@param near number
---@param far number
---@return Matrix4x4
function glm.perspective(fov, aspect, near, far) end

---@param left number
---@param right number
---@param bottom number
---@param top number
---@param near number
---@param far number
---@return Matrix4x4
function glm.ortho(left, right, bottom, top, near, far) end

---@param eye Vector3
---@param center Vector3
---@param up Vector3
---@return Matrix4x4
function glm.lookAt(eye, center, up) end

---@param position Vector3
---@param rotation Vector3
---@return Matrix4x4
function glm.translate(position, rotation) end

---@param axis Vector3
---@param angle number
---@return Matrix4x4
function glm.rotate(axis, angle) end

---@param factors Vector3
---@return Matrix4x4
function glm.scale(factors) end

---@param quat Quat
---@return Matrix4x4
function glm.toMat4(quat) end

---@param m Matrix4x4
---@return Quat
function glm.toQuat(m) end

---@param angle number
---@param axis Vector3
---@return Quat
function glm.angleAxis(angle, axis) end

---@param eulerAngles Vector3
---@return Quat
function glm.euler(eulerAngles) end

-- Geometry API (AABB, Ray, Sphere, Plane, Polygon)
-- Declared as stub types for hover/completion support.

---@class AABB
---@field min Vector3
---@field max Vector3
local AABB = {}

---@param min Vector3
---@param max Vector3
---@return AABB
function glm.aabb(min, max) end

---@class Ray
---@field origin Vector3
---@field direction Vector3
local Ray = {}

---@param origin Vector3
---@param direction Vector3
---@return Ray
function glm.ray(origin, direction) end

---@class Sphere
---@field center Vector3
---@field radius number
local Sphere = {}

---@param center Vector3
---@param radius number
---@return Sphere
function glm.sphere(center, radius) end

---@class Plane
---@field normal Vector3
---@field distance number
local Plane = {}

---@param normal Vector3
---@param distance number
---@return Plane
function glm.plane(normal, distance) end

---@class Polygon
---@field points Vector3[]
local Polygon = {}

---@param points Vector3[]
---@return Polygon
function glm.polygon(points) end
