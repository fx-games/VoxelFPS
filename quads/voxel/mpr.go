package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "math"
)

type CCDSupport struct {
    v  mgl32.Vec3 // support point in minkowski difference
    v1 mgl32.Vec3 // support vertex of obj1
    v2 mgl32.Vec3 // support vertex of obj2
}

type CCDSimplex struct {
    ps   [4]CCDSupport
    last int // index of last added point
}

func NewCCDSimplex() *CCDSimplex {
    return &CCDSimplex{
        last: -1,
    }
}

func (s *CCDSimplex) Size() int {
    return s.last + 1
}

/*

void ccdSupport(const void *_obj, const ccd_vec3_t *_dir,
                ccd_vec3_t *v)
{
    // Support function is made according to Gino van den Bergen's paper
    //  A Fast and Robust CCD Implementation for Collision Detection of
    //  Convex Objects

    ccd_obj_t *obj = (ccd_obj_t *)_obj;
    ccd_vec3_t dir;
    ccd_quat_t qinv;

    ccdVec3Copy(&dir, _dir);
    ccdQuatInvert2(&qinv, &obj->quat);

    ccdQuatRotVec(&dir, &qinv);

    if (obj->type == CCD_OBJ_BOX){
        ccd_box_t *box = (ccd_box_t *)obj;
        ccdVec3Set(v, ccdSign(ccdVec3X(&dir)) * box->x * CCD_REAL(0.5),
                      ccdSign(ccdVec3Y(&dir)) * box->y * CCD_REAL(0.5),
                      ccdSign(ccdVec3Z(&dir)) * box->z * CCD_REAL(0.5));
    }else if (obj->type == CCD_OBJ_SPHERE){
        ccd_sphere_t *sphere = (ccd_sphere_t *)obj;
        ccd_real_t len;

        len = ccdVec3Len2(&dir);
        if (len - CCD_EPS > CCD_ZERO){
            ccdVec3Copy(v, &dir);
            ccdVec3Scale(v, sphere->radius / CCD_SQRT(len));
        }else{
            ccdVec3Set(v, CCD_ZERO, CCD_ZERO, CCD_ZERO);
        }
    }else if (obj->type == CCD_OBJ_CYL){
        ccd_cyl_t *cyl = (ccd_cyl_t *)obj;
        ccd_real_t zdist, rad;

        zdist = dir.v[0] * dir.v[0] + dir.v[1] * dir.v[1];
        zdist = CCD_SQRT(zdist);
        if (ccdIsZero(zdist)){
            ccdVec3Set(v, CCD_ZERO, CCD_ZERO,
                          ccdSign(ccdVec3Z(&dir)) * cyl->height * CCD_REAL(0.5));
        }else{
            rad = cyl->radius / zdist;

            ccdVec3Set(v, rad * ccdVec3X(&dir),
                          rad * ccdVec3Y(&dir),
                          ccdSign(ccdVec3Z(&dir)) * cyl->height * CCD_REAL(0.5));
        }
    }

    // transform support vertex
    ccdQuatRotVec(v, &obj->quat);
    ccdVec3Add(v, &obj->pos);
}
*/

type Box struct {
    SizeX, SizeY, SizeZ float32
    Transform           *core.Transform
}

func NewBox(sizeX, sizeY, sizeZ float32) Box {
    return Box{
        SizeX:     sizeX,
        SizeY:     sizeY,
        SizeZ:     sizeZ,
        Transform: core.NewDefaultTransform(),
    }
}

func ccdSupport(obj Box, dir mgl32.Vec3) (v mgl32.Vec3) {
    // Support function is made according to Gino van den Bergen's paper
    //  A Fast and Robust CCD Implementation for Collision Detection of
    //  Convex Objects

    var qinv mgl32.Quat

    dir = dir.Normalize()
    qinv = obj.Transform.GetRotation().Conjugate()

    dir = qinv.Rotate(dir)

    v = mgl32.Vec3{
        ccdSign(dir.X()) * obj.SizeX * 0.5,
        ccdSign(dir.Y()) * obj.SizeY * 0.5,
        ccdSign(dir.Z()) * obj.SizeZ * 0.5,
    }

    // transform support vertex
    v = obj.Transform.GetRotation().Rotate(v)
    v = v.Add(obj.Transform.GetPosition())

    return
}

/*
_ccd_inline int ccdSign(ccd_real_t val)
{
    if (ccdIsZero(val)){
        return 0;
    }else if (val < CCD_ZERO){
        return -1;
    }
    return 1;
}
*/
func ccdSign(x float32) float32 {
    if x == 0 {
        return 0
    } else if x < 0 {
        return -1
    }
    return 1
}

/*
void ccdObjCenter(const void *_obj, ccd_vec3_t *center)
{
    ccd_obj_t *obj = (ccd_obj_t *)_obj;

    ccdVec3Set(center, CCD_ZERO, CCD_ZERO, CCD_ZERO);
    // rotation is not needed
    ccdVec3Add(center, &obj->pos);
}
*/

func ccdObjCenter(obj Box) mgl32.Vec3 {
    return obj.Transform.GetPosition()
}

/*
int ccdMPRIntersect(const void *obj1, const void *obj2, const ccd_t *ccd)
{
    ccd_simplex_t portal;
    int res;

    // Phase 1: Portal discovery - find portal that intersects with origin
    // ray (ray from center of Minkowski diff to origin of coordinates)
    res = discoverPortal(obj1, obj2, ccd, &portal);
    if (res < 0)
        return 0;
    if (res > 0)
        return 1;

    // Phase 2: Portal refinement
    res = refinePortal(obj1, obj2, ccd, &portal);
    return (res == 0 ? 1 : 0);
}
*/
func ccdMPRIntersect(obj1, obj2 Box) (res bool) {
    portal, portalFound := discoverPortal(obj1, obj2)
    if portalFound < 0 {
        return false
    }
    if portalFound > 0 {
        return true
    }
    res = refinePortal(obj1, obj2, portal)
    return !res
}

/*
_ccd_inline void findOrigin(const void *obj1, const void *obj2, const ccd_t *ccd,
                            ccd_support_t *center)
{
    ccd->center1(obj1, &center->v1);
    ccd->center2(obj2, &center->v2);
    ccdVec3Sub2(&center->v, &center->v1, &center->v2);
}
*/

func findOrigin(obj1, obj2 Box) mgl32.Vec3 {
    return ccdObjCenter(obj1).Sub(ccdObjCenter(obj2))
}

/*
static int discoverPortal(const void *obj1, const void *obj2,
                          const ccd_t *ccd, ccd_simplex_t *portal)
{
    ccd_vec3_t dir, va, vb;
    ccd_real_t dot;
    int cont;

    // vertex 0 is center of portal
    findOrigin(obj1, obj2, ccd, ccdSimplexPointW(portal, 0));
    ccdSimplexSetSize(portal, 1);

    if (ccdVec3Eq(&ccdSimplexPoint(portal, 0)->v, ccd_vec3_origin)){
        // Portal's center lies on origin (0,0,0) => we know that objects
        // intersect but we would need to know penetration info.
        // So move center little bit...
        ccdVec3Set(&va, CCD_EPS * CCD_REAL(10.), CCD_ZERO, CCD_ZERO);
        ccdVec3Add(&ccdSimplexPointW(portal, 0)->v, &va);
    }


    // vertex 1 = support in direction of origin
    ccdVec3Copy(&dir, &ccdSimplexPoint(portal, 0)->v);
    ccdVec3Scale(&dir, CCD_REAL(-1.));
    ccdVec3Normalize(&dir);
    __ccdSupport(obj1, obj2, &dir, ccd, ccdSimplexPointW(portal, 1));
    ccdSimplexSetSize(portal, 2);

    // test if origin isn't outside of v1
    dot = ccdVec3Dot(&ccdSimplexPoint(portal, 1)->v, &dir);
    if (ccdIsZero(dot) || dot < CCD_ZERO)
        return -1;


    // vertex 2
    ccdVec3Cross(&dir, &ccdSimplexPoint(portal, 0)->v,
                       &ccdSimplexPoint(portal, 1)->v);
    if (ccdIsZero(ccdVec3Len2(&dir))){
        if (ccdVec3Eq(&ccdSimplexPoint(portal, 1)->v, ccd_vec3_origin)){
            // origin lies on v1
            return 1;
        }else{
            // origin lies on v0-v1 segment
            return 2;
        }
    }

    ccdVec3Normalize(&dir);
    __ccdSupport(obj1, obj2, &dir, ccd, ccdSimplexPointW(portal, 2));
    dot = ccdVec3Dot(&ccdSimplexPoint(portal, 2)->v, &dir);
    if (ccdIsZero(dot) || dot < CCD_ZERO)
        return -1;

    ccdSimplexSetSize(portal, 3);

    // vertex 3 direction
    ccdVec3Sub2(&va, &ccdSimplexPoint(portal, 1)->v,
                     &ccdSimplexPoint(portal, 0)->v);
    ccdVec3Sub2(&vb, &ccdSimplexPoint(portal, 2)->v,
                     &ccdSimplexPoint(portal, 0)->v);
    ccdVec3Cross(&dir, &va, &vb);
    ccdVec3Normalize(&dir);

    // it is better to form portal faces to be oriented "outside" origin
    dot = ccdVec3Dot(&dir, &ccdSimplexPoint(portal, 0)->v);
    if (dot > CCD_ZERO){
        ccdSimplexSwap(portal, 1, 2);
        ccdVec3Scale(&dir, CCD_REAL(-1.));
    }

    while (ccdSimplexSize(portal) < 4){
        __ccdSupport(obj1, obj2, &dir, ccd, ccdSimplexPointW(portal, 3));
        dot = ccdVec3Dot(&ccdSimplexPoint(portal, 3)->v, &dir);
        if (ccdIsZero(dot) || dot < CCD_ZERO)
            return -1;

        cont = 0;

        // test if origin is outside (v1, v0, v3) - set v2 as v3 and
        // continue
        ccdVec3Cross(&va, &ccdSimplexPoint(portal, 1)->v,
                          &ccdSimplexPoint(portal, 3)->v);
        dot = ccdVec3Dot(&va, &ccdSimplexPoint(portal, 0)->v);
        if (dot < CCD_ZERO && !ccdIsZero(dot)){
            ccdSimplexSet(portal, 2, ccdSimplexPoint(portal, 3));
            cont = 1;
        }

        if (!cont){
            // test if origin is outside (v3, v0, v2) - set v1 as v3 and
            // continue
            ccdVec3Cross(&va, &ccdSimplexPoint(portal, 3)->v,
                              &ccdSimplexPoint(portal, 2)->v);
            dot = ccdVec3Dot(&va, &ccdSimplexPoint(portal, 0)->v);
            if (dot < CCD_ZERO && !ccdIsZero(dot)){
                ccdSimplexSet(portal, 1, ccdSimplexPoint(portal, 3));
                cont = 1;
            }
        }

        if (cont){
            ccdVec3Sub2(&va, &ccdSimplexPoint(portal, 1)->v,
                             &ccdSimplexPoint(portal, 0)->v);
            ccdVec3Sub2(&vb, &ccdSimplexPoint(portal, 2)->v,
                             &ccdSimplexPoint(portal, 0)->v);
            ccdVec3Cross(&dir, &va, &vb);
            ccdVec3Normalize(&dir);
        }else{
            ccdSimplexSetSize(portal, 4);
        }
    }

    return 0;
}
*/

func discoverPortal(obj1, obj2 Box) (portal CCDSimplex, portalFound int) {
    originVector := mgl32.Vec3{0, 0, 0}

    var dir, va, vb mgl32.Vec3
    var dot float32

    // vertex 0 is center of portal
    portal.ps[0].v = findOrigin(obj1, obj2)
    portal.last = 0

    if portal.ps[0].v == originVector {
        // Portal's center lies on origin (0,0,0) => we know that objects
        // intersect but we would need to know penetration info.
        // So move center little bit...
        va = mgl32.Vec3{0.0001 * 10, 0, 0}
        portal.ps[0].v = portal.ps[0].v.Add(va)
    }

    // vertex 1 = support in direction of origin
    dir = portal.ps[0].v.Mul(-1).Normalize()
    portal.ps[1].v = ccdSupport(obj1, dir)
    portal.last = 1

    // test if origin isn't outside of v1
    dot = portal.ps[1].v.Dot(dir)
    if ccdIsZero(dot) || dot < 0 {
        return portal, -1
    }

    // vertex 2
    dir = portal.ps[0].v.Cross(portal.ps[1].v).Normalize()
    if dir.Len() == 0 {
        if portal.ps[1].v == originVector {
            // origin lies on v1
            return portal, 1
        } else {
            // origin lies on v0-v1 segment
            return portal, 2
        }
    }

    portal.ps[2].v = ccdSupport(obj1, dir)
    dot = portal.ps[2].v.Dot(dir)
    if ccdIsZero(dot) || dot < 0 {
        return portal, -1
    }
    portal.last = 2

    // vertex 3 direction
    va = portal.ps[1].v.Sub(portal.ps[0].v)
    vb = portal.ps[2].v.Sub(portal.ps[0].v)
    dir = va.Cross(vb).Normalize()

    // it is better to form portal faces to be oriented "outside" origin
    dot = dir.Dot(portal.ps[0].v)
    if dot > 0 {
        portal.ps[1], portal.ps[2] = portal.ps[2], portal.ps[1]
        dir = dir.Mul(-1)
    }

    for portal.Size() < 4 {
        portal.ps[3].v = ccdSupport(obj1, dir)
        dot = portal.ps[3].v.Dot(dir)
        if dot == 0 || dot < 0 {
            return portal, -1
        }

        cont := false

        // test if origin is outside (v1, v0, v3) - set v2 as v3 and
        // continue
        va = portal.ps[1].v.Cross(portal.ps[3].v)
        dot = va.Dot(portal.ps[0].v)
        if dot < 0 && !ccdIsZero(dot) {
            portal.ps[2] = portal.ps[3]
            cont = true
        }

        if !cont {
            // test if origin is outside (v3, v0, v2) - set v1 as v3 and
            // continue
            va = portal.ps[3].v.Cross(portal.ps[2].v)
            dot = va.Dot(portal.ps[0].v)
            if dot < 0 && !ccdIsZero(dot) {
                portal.ps[1] = portal.ps[3]
                cont = true
            }
        }

        if cont {
            va = portal.ps[1].v.Sub(portal.ps[0].v)
            vb = portal.ps[2].v.Sub(portal.ps[0].v)
            dir = va.Cross(vb).Normalize()
        } else {
            portal.last = 4
        }
    }

    return portal, 0
}

/*
_ccd_inline void portalDir(const ccd_simplex_t *portal, ccd_vec3_t *dir)
{
    ccd_vec3_t v2v1, v3v1;

    ccdVec3Sub2(&v2v1, &ccdSimplexPoint(portal, 2)->v,
                       &ccdSimplexPoint(portal, 1)->v);
    ccdVec3Sub2(&v3v1, &ccdSimplexPoint(portal, 3)->v,
                       &ccdSimplexPoint(portal, 1)->v);
    ccdVec3Cross(dir, &v2v1, &v3v1);
    ccdVec3Normalize(dir);
}

_ccd_inline int portalEncapsulesOrigin(const ccd_simplex_t *portal,
                                       const ccd_vec3_t *dir)
{
    ccd_real_t dot;
    dot = ccdVec3Dot(dir, &ccdSimplexPoint(portal, 1)->v);
    return ccdIsZero(dot) || dot > CCD_ZERO;
}

_ccd_inline int portalReachTolerance(const ccd_simplex_t *portal,
                                     const ccd_support_t *v4,
                                     const ccd_vec3_t *dir,
                                     const ccd_t *ccd)
{
    ccd_real_t dv1, dv2, dv3, dv4;
    ccd_real_t dot1, dot2, dot3;

    // find the smallest dot product of dir and {v1-v4, v2-v4, v3-v4}

    dv1 = ccdVec3Dot(&ccdSimplexPoint(portal, 1)->v, dir);
    dv2 = ccdVec3Dot(&ccdSimplexPoint(portal, 2)->v, dir);
    dv3 = ccdVec3Dot(&ccdSimplexPoint(portal, 3)->v, dir);
    dv4 = ccdVec3Dot(&v4->v, dir);

    dot1 = dv4 - dv1;
    dot2 = dv4 - dv2;
    dot3 = dv4 - dv3;

    dot1 = CCD_FMIN(dot1, dot2);
    dot1 = CCD_FMIN(dot1, dot3);

    return ccdEq(dot1, ccd->mpr_tolerance) || dot1 < ccd->mpr_tolerance;
}

_ccd_inline int portalCanEncapsuleOrigin(const ccd_simplex_t *portal,
                                         const ccd_support_t *v4,
                                         const ccd_vec3_t *dir)
{
    ccd_real_t dot;
    dot = ccdVec3Dot(&v4->v, dir);
    return ccdIsZero(dot) || dot > CCD_ZERO;
}
*/
/*
_ccd_inline int ccdEq(ccd_real_t _a, ccd_real_t _b)
{
    ccd_real_t ab;
    ccd_real_t a, b;

    ab = CCD_FABS(_a - _b);
    if (CCD_FABS(ab) < CCD_EPS)
        return 1;

    a = CCD_FABS(_a);
    b = CCD_FABS(_b);
    if (b > a){
        return ab < CCD_EPS * b;
    }else{
        return ab < CCD_EPS * a;
    }
}
*/
const CcdEps = 0.0000000001

func ccdEq(a, b float64) bool {
    ab := a - b
    if ab < CcdEps {
        return true
    }

    a = math.Abs(a)
    b = math.Abs(b)
    if b > a {
        return ab < CcdEps*b
    } else {
        return ab < CcdEps*a
    }
}

func ccdIsZero(val float32) bool {
    return math.Abs(float64(val)) < CcdEps
}

func portalReachTolerance(portal CCDSimplex, v4 CCDSupport, dir mgl32.Vec3) bool {
    tolerance := float32(0.0001)

    dv1 := portal.ps[1].v.Dot(dir)
    dv2 := portal.ps[2].v.Dot(dir)
    dv3 := portal.ps[3].v.Dot(dir)
    dv4 := v4.v.Dot(dir)

    dot1 := dv4 - dv1
    dot2 := dv4 - dv2
    dot3 := dv4 - dv3

    dot1 = min(dot1, dot2)
    dot1 = min(dot1, dot3)

    return ccdEq(float64(dot1), float64(tolerance)) || dot1 < tolerance
}

func portalDir(portal CCDSimplex) mgl32.Vec3 {
    v2v1 := portal.ps[2].v.Sub(portal.ps[1].v)
    v3v1 := portal.ps[3].v.Sub(portal.ps[1].v)
    return v2v1.Cross(v3v1).Normalize()
}

func portalEncapsulesOrigin(portal CCDSimplex, dir mgl32.Vec3) bool {
    dot := dir.Dot(portal.ps[1].v)
    return ccdIsZero(dot) || dot > 0
}

/*
_ccd_inline int portalCanEncapsuleOrigin(const ccd_simplex_t *portal,
                                         const ccd_support_t *v4,
                                         const ccd_vec3_t *dir)
{
    ccd_real_t dot;
    dot = ccdVec3Dot(&v4->v, dir);
    return ccdIsZero(dot) || dot > CCD_ZERO;
}
*/

func portalCanEncapsuleOrigin(v4 CCDSupport, dir mgl32.Vec3) bool {
    dot := v4.v.Dot(dir)
    return ccdIsZero(dot) || dot > 0
}

/*

void __ccdSupport(const void *obj1, const void *obj2,
                  const ccd_vec3_t *_dir, const ccd_t *ccd,
                  ccd_support_t *supp)
{
    ccd_vec3_t dir;

    ccdVec3Copy(&dir, _dir);

    ccd->support1(obj1, &dir, &supp->v1);

    ccdVec3Scale(&dir, -CCD_ONE);
    ccd->support2(obj2, &dir, &supp->v2);

    ccdVec3Sub2(&supp->v, &supp->v1, &supp->v2);
}
*/

func calculateSupports(obj1, obj2 Box, dir mgl32.Vec3) CCDSupport {
    var supp CCDSupport
    supp.v1 = ccdSupport(obj1, dir)
    supp.v2 = ccdSupport(obj2, dir.Mul(-1))
    supp.v = supp.v1.Sub(supp.v2)
    return supp
}

/*
static int refinePortal(const void *obj1, const void *obj2,
                        const ccd_t *ccd, ccd_simplex_t *portal)
{
    ccd_vec3_t dir;
    ccd_support_t v4;

    while (1){
        // compute direction outside the portal (from v0 throught v1,v2,v3
        // face)
        portalDir(portal, &dir);

        // test if origin is inside the portal
        if (portalEncapsulesOrigin(portal, &dir))
            return 0;

        // get next support point
        __ccdSupport(obj1, obj2, &dir, ccd, &v4);

        // test if v4 can expand portal to contain origin and if portal
        // expanding doesn't reach given tolerance
        if (!portalCanEncapsuleOrigin(portal, &v4, &dir)
                || portalReachTolerance(portal, &v4, &dir, ccd)){
            return -1;
        }

        // v1-v2-v3 triangle must be rearranged to face outside Minkowski
        // difference (direction from v0).
        expandPortal(portal, &v4);
    }

    return -1;
}
*/
func refinePortal(obj1, obj2 Box, portal CCDSimplex) bool {
    var dir mgl32.Vec3
    var v4 CCDSupport

    for {
        // compute direction outside the portal (from v0 throught v1,v2,v3
        // face)
        dir = portalDir(portal)

        // test if origin is inside the portal
        if portalEncapsulesOrigin(portal, dir) {
            return true
        }

        // get next support point
        v4 = calculateSupports(obj1, obj2, dir)

        // test if v4 can expand portal to contain origin and if portal
        // expanding doesn't reach given tolerance
        if !portalCanEncapsuleOrigin(v4, dir) || portalReachTolerance(portal, v4, dir) {
            return false
        }

        // v1-v2-v3 triangle must be rearranged to face outside Minkowski
        // difference (direction from v0).
        expandPortal(&portal, v4)
    }

    return false

}

/*
_ccd_inline void expandPortal(ccd_simplex_t *portal,
                              const ccd_support_t *v4)
{
    ccd_real_t dot;
    ccd_vec3_t v4v0;

    ccdVec3Cross(&v4v0, &v4->v, &ccdSimplexPoint(portal, 0)->v);
    dot = ccdVec3Dot(&ccdSimplexPoint(portal, 1)->v, &v4v0);
    if (dot > CCD_ZERO){
        dot = ccdVec3Dot(&ccdSimplexPoint(portal, 2)->v, &v4v0);
        if (dot > CCD_ZERO){
            ccdSimplexSet(portal, 1, v4);
        }else{
            ccdSimplexSet(portal, 3, v4);
        }
    }else{
        dot = ccdVec3Dot(&ccdSimplexPoint(portal, 3)->v, &v4v0);
        if (dot > CCD_ZERO){
            ccdSimplexSet(portal, 2, v4);
        }else{
            ccdSimplexSet(portal, 1, v4);
        }
    }
}

*/

func expandPortal(portal *CCDSimplex, v4 CCDSupport) {
    var v4v0 mgl32.Vec3
    v4v0 = v4.v.Cross(portal.ps[0].v)
    dot := portal.ps[1].v.Dot(v4v0)
    if dot > 0 {
        dot = portal.ps[2].v.Dot(v4v0)
        if dot > 0 {
            portal.ps[1] = v4
        } else {
            portal.ps[3] = v4
        }
    } else {
        dot = portal.ps[3].v.Dot(v4v0)
        if dot > 0 {
            portal.ps[2] = v4
        } else {
            portal.ps[1] = v4
        }
    }
}
