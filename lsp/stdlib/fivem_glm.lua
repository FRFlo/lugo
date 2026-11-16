---@meta
-- FiveM / LuaGLM — Vector, Matrix, and Quaternion type declarations.
-- These types exist as first-class types in the FiveM runtime (type 4 = vector, type 10 = matrix).
-- The Lua 5.4 type number for string is shifted from 4 to 5, and all subsequent types shift by +1.

-- =============================================================================
-- Vector Types
-- =============================================================================

---@class VectorOnlyMethods
local VectorOnlyMethods = {}

---@return number
function VectorOnlyMethods:length() end

---@return number
function VectorOnlyMethods:length2() end

---@class Vector2: VectorOnlyMethods
---@field x number
---@field y number
local Vector2 = {}

---@param other Vector2
---@return number
function Vector2:dot(other) end

---@return Vector2
function Vector2:normalize() end

---@param target Vector2
---@param t number
---@return Vector2
function Vector2:lerp(target, t) end

---@param other Vector2
---@return number
function Vector2:cross(other) end

---@param min? number|Vector2
---@param max? number|Vector2
---@return Vector2
function Vector2:clamp(min, max) end

---@class Vector3: VectorOnlyMethods
---@field x number
---@field y number
---@field z number
local Vector3 = {}

---@param other Vector3
---@return number
function Vector3:dot(other) end

---@return Vector3
function Vector3:normalize() end

---@param target Vector3
---@param t number
---@return Vector3
function Vector3:lerp(target, t) end

---@param other Vector3|Quat
---@return Vector3
function Vector3:cross(other) end

---@param min? number|Vector3
---@param max? number|Vector3
---@return Vector3
function Vector3:clamp(min, max) end

---@class Vector4: VectorOnlyMethods
---@field x number
---@field y number
---@field z number
---@field w number
local Vector4 = {}

---@param other Vector4
---@return number
function Vector4:dot(other) end

---@return Vector4
function Vector4:normalize() end

---@param target Vector4
---@param t number
---@return Vector4
function Vector4:lerp(target, t) end

---@param min? number|Vector4
---@param max? number|Vector4
---@return Vector4
function Vector4:clamp(min, max) end

-- =============================================================================
-- Quaternion Type
-- =============================================================================

---@class Quat
---@field x number
---@field y number
---@field z number
---@field w number
local Quat = {}

---@param target Quat
---@param t number
---@return Quat
function Quat:lerp(target, t) end

---@param other Vector3|Quat
---@return Vector3|Quat
function Quat:cross(other) end

---@return Quat
function Quat:inverse() end

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

---@overload fun(x: number): number
---@overload fun(x: number, y: number): Vector2
---@overload fun(x: number, y: number, z: number): Vector3
---@overload fun(x: number, y: number, z: number, w: number): Vector4
---@param ... number|Vector2|Vector3|Vector4|Quat|number[]
---@return number|Vector2|Vector3|Vector4
function vec(...) end
---@overload fun(x: number): number
---@overload fun(x: number, y: number): Vector2
---@overload fun(x: number, y: number, z: number): Vector3
---@overload fun(x: number, y: number, z: number, w: number): Vector4
---@param ... number|Vector2|Vector3|Vector4|Quat|number[]
---@return number|Vector2|Vector3|Vector4
function vector(...) end
function vec1() end
function vector1() end

---@param x? number
---@param y? number
---@return Vector2
function vector2(x, y) end

---@param x? number
---@param y? number
---@param z? number
---@return Vector3
function vector3(x, y, z) end

---@param x? number
---@param y? number
---@param z? number
---@param w? number
---@return Vector4
function vector4(x, y, z, w) end

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

---@overload fun(a: Vector3, b: Vector3): number
---@overload fun(a: Vector4, b: Vector4): number
---@param a Vector2
---@param b Vector2
---@return number
function dot(a, b) end

---@overload fun(a: Vector3, b: Quat): Vector3
---@overload fun(a: Quat, b: Quat): Quat
---@overload fun(a: Quat, b: Vector3): Vector3
---@overload fun(a: Vector3, b: Vector3): Vector3
---@param a Vector2
---@param b Vector2
---@return number
function cross(a, b) end

---@param v Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function inv(v) end

---@param v Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function norm(v) end

---@overload fun(a: Vector2|Vector3|Vector4, b: Vector2|Vector3|Vector4, t: number): Vector2|Vector3|Vector4
---@param a Quat
---@param b Quat
---@param t number
---@return Quat
function slerp(a, b, t) end

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

-- Runtime constructors registered by cfxlua-glm. Keep the declared shape
-- types aligned with the global constructor declarations above.
---@overload fun(x: number): number
---@overload fun(x: number, y: number): Vector2
---@overload fun(x: number, y: number, z: number): Vector3
---@overload fun(x: number, y: number, z: number, w: number): Vector4
---@param ... number|Vector2|Vector3|Vector4|Quat|number[]
---@return number|Vector2|Vector3|Vector4
function glm.vec(...) end
function glm.vec1() end
---@param x? number
---@return Vector2
function glm.vec2(x) end
---@overload fun(x: number, y: number, z: number): Vector3
---@param x? number
---@param y? number
---@return Vector3
function glm.vec3(x, y) end
---@overload fun(x: number, y: number, z: number, w: number): Vector4
---@param x? number
---@param y? number
---@param z? number
---@return Vector4
function glm.vec4(x, y, z) end

---@return Vector2
function glm.ivec(x) end
function glm.ivec1() end
function glm.ivec2() end
function glm.ivec3() end
function glm.ivec4() end

---@return Vector2
function glm.bvec(x) end
function glm.bvec1() end
function glm.bvec2() end
function glm.bvec3() end
function glm.bvec4() end

---@return Matrix4x4
function glm.mat() end
---@return Matrix2x2
function glm.mat2x2() end
---@return Matrix2x2
function glm.mat2() end
---@return Matrix2x3
function glm.mat2x3() end
---@return Matrix2x4
function glm.mat2x4() end
---@return Matrix3x2
function glm.mat3x2() end
---@return Matrix3x3
function glm.mat3x3() end
---@return Matrix3x3
function glm.mat3() end
---@return Matrix3x4
function glm.mat3x4() end
---@return Matrix4x2
function glm.mat4x2() end
---@return Matrix4x3
function glm.mat4x3() end
---@return Matrix4x4
function glm.mat4x4() end
---@return Matrix4x4
function glm.mat4() end

---@return Quat
function glm.qua() end
---@return Quat
function glm.quat() end

-- Vector utilities
---@param v Vector2|Vector3|Vector4
---@return number
function glm.length(v) end

---@param a Vector2|Vector3|Vector4
---@param b Vector2|Vector3|Vector4
---@return number
function glm.distance(a, b) end

---@overload fun(a: Vector3, b: Vector3): number
---@overload fun(a: Vector4, b: Vector4): number
---@param a Vector2
---@param b Vector2
---@return number
function glm.dot(a, b) end

---@overload fun(a: Vector3, b: Quat): Vector3
---@overload fun(a: Quat, b: Quat): Quat
---@overload fun(a: Quat, b: Vector3): Vector3
---@overload fun(a: Vector3, b: Vector3): Vector3
---@param a Vector2
---@param b Vector2
---@return number
function glm.cross(a, b) end

---@param v Vector2|Vector3|Vector4
---@return Vector2|Vector3|Vector4
function glm.normalize(v) end

---@overload fun(m: Quat): Quat
---@overload fun(m: Matrix2x2): Matrix2x2
---@overload fun(m: Matrix3x3): Matrix3x3
---@overload fun(m: Matrix4x4): Matrix4x4
---@generic T
---@param m T
---@return T
function glm.inverse(m) end

---@overload fun(a: Vector2|Vector3|Vector4, b: Vector2|Vector3|Vector4, t: number): Vector2|Vector3|Vector4
---@param a Quat
---@param b Quat
---@param t number
---@return Quat
function glm.slerp(a, b, t) end

---@overload fun(v: Vector2): Vector2
---@overload fun(v: Vector2, min: Vector2, max: Vector2): Vector2
---@overload fun(v: Vector3, min: Vector3, max: Vector3): Vector3
---@overload fun(v: Vector4, min: Vector4, max: Vector4): Vector4
---@param v Vector2|Vector3|Vector4
---@param min? number
---@param max? number
---@return Vector2|Vector3|Vector4
function glm.clamp(v, min, max) end

---@overload fun(a: Quat, b: Quat, t: number): Quat
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

---@overload fun(translation: Vector3): Matrix4x4
---@overload fun(m: Matrix3x3, translation: Vector2): Matrix3x3
---@param m Matrix4x4
---@param translation Vector3
---@return Matrix4x4
function glm.translate(m, translation) end

---@overload fun(angle: number, axis: Vector3): Matrix4x4
---@overload fun(m: Matrix3x3, angle: number): Matrix3x3
---@param m Matrix4x4
---@param angle number
---@param axis Vector3
---@return Matrix4x4
function glm.rotate(m, angle, axis) end

---@overload fun(factors: Vector3): Matrix4x4
---@overload fun(m: Matrix3x3, factors: Vector2): Matrix3x3
---@param m Matrix4x4
---@param factors Vector3
---@return Matrix4x4
function glm.scale(m, factors) end

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

glm.polygon = {}
---@param points Vector3[]
---@return Polygon
function glm.polygon.new(points) end
