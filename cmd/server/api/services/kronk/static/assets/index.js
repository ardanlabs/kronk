var mu=Object.defineProperty;var pu=(n,t,r)=>t in n?mu(n,t,{enumerable:!0,configurable:!0,writable:!0,value:r}):n[t]=r;var mo=(n,t,r)=>pu(n,typeof t!="symbol"?t+"":t,r);function fu(n,t){for(var r=0;r<t.length;r++){const s=t[r];if(typeof s!="string"&&!Array.isArray(s)){for(const l in s)if(l!=="default"&&!(l in n)){const i=Object.getOwnPropertyDescriptor(s,l);i&&Object.defineProperty(n,l,i.get?i:{enumerable:!0,get:()=>s[l]})}}}return Object.freeze(Object.defineProperty(n,Symbol.toStringTag,{value:"Module"}))}(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const l of document.querySelectorAll('link[rel="modulepreload"]'))s(l);new MutationObserver(l=>{for(const i of l)if(i.type==="childList")for(const o of i.addedNodes)o.tagName==="LINK"&&o.rel==="modulepreload"&&s(o)}).observe(document,{childList:!0,subtree:!0});function r(l){const i={};return l.integrity&&(i.integrity=l.integrity),l.referrerPolicy&&(i.referrerPolicy=l.referrerPolicy),l.crossOrigin==="use-credentials"?i.credentials="include":l.crossOrigin==="anonymous"?i.credentials="omit":i.credentials="same-origin",i}function s(l){if(l.ep)return;l.ep=!0;const i=r(l);fetch(l.href,i)}})();var po=typeof globalThis<"u"?globalThis:typeof window<"u"?window:typeof global<"u"?global:typeof self<"u"?self:{};function La(n){return n&&n.__esModule&&Object.prototype.hasOwnProperty.call(n,"default")?n.default:n}var Fa={exports:{}},ws={},Da={exports:{}},D={};/**
 * @license React
 * react.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var fr=Symbol.for("react.element"),xu=Symbol.for("react.portal"),ju=Symbol.for("react.fragment"),gu=Symbol.for("react.strict_mode"),vu=Symbol.for("react.profiler"),yu=Symbol.for("react.provider"),ku=Symbol.for("react.context"),Nu=Symbol.for("react.forward_ref"),bu=Symbol.for("react.suspense"),wu=Symbol.for("react.memo"),Eu=Symbol.for("react.lazy"),fo=Symbol.iterator;function Su(n){return n===null||typeof n!="object"?null:(n=fo&&n[fo]||n["@@iterator"],typeof n=="function"?n:null)}var Ua={isMounted:function(){return!1},enqueueForceUpdate:function(){},enqueueReplaceState:function(){},enqueueSetState:function(){}},Ka=Object.assign,Ba={};function wt(n,t,r){this.props=n,this.context=t,this.refs=Ba,this.updater=r||Ua}wt.prototype.isReactComponent={};wt.prototype.setState=function(n,t){if(typeof n!="object"&&typeof n!="function"&&n!=null)throw Error("setState(...): takes an object of state variables to update or a function which returns an object of state variables.");this.updater.enqueueSetState(this,n,t,"setState")};wt.prototype.forceUpdate=function(n){this.updater.enqueueForceUpdate(this,n,"forceUpdate")};function za(){}za.prototype=wt.prototype;function fi(n,t,r){this.props=n,this.context=t,this.refs=Ba,this.updater=r||Ua}var xi=fi.prototype=new za;xi.constructor=fi;Ka(xi,wt.prototype);xi.isPureReactComponent=!0;var xo=Array.isArray,$a=Object.prototype.hasOwnProperty,ji={current:null},Ha={key:!0,ref:!0,__self:!0,__source:!0};function qa(n,t,r){var s,l={},i=null,o=null;if(t!=null)for(s in t.ref!==void 0&&(o=t.ref),t.key!==void 0&&(i=""+t.key),t)$a.call(t,s)&&!Ha.hasOwnProperty(s)&&(l[s]=t[s]);var a=arguments.length-2;if(a===1)l.children=r;else if(1<a){for(var c=Array(a),u=0;u<a;u++)c[u]=arguments[u+2];l.children=c}if(n&&n.defaultProps)for(s in a=n.defaultProps,a)l[s]===void 0&&(l[s]=a[s]);return{$$typeof:fr,type:n,key:i,ref:o,props:l,_owner:ji.current}}function Tu(n,t){return{$$typeof:fr,type:n.type,key:t,ref:n.ref,props:n.props,_owner:n._owner}}function gi(n){return typeof n=="object"&&n!==null&&n.$$typeof===fr}function Ru(n){var t={"=":"=0",":":"=2"};return"$"+n.replace(/[=:]/g,function(r){return t[r]})}var jo=/\/+/g;function Hs(n,t){return typeof n=="object"&&n!==null&&n.key!=null?Ru(""+n.key):t.toString(36)}function Br(n,t,r,s,l){var i=typeof n;(i==="undefined"||i==="boolean")&&(n=null);var o=!1;if(n===null)o=!0;else switch(i){case"string":case"number":o=!0;break;case"object":switch(n.$$typeof){case fr:case xu:o=!0}}if(o)return o=n,l=l(o),n=s===""?"."+Hs(o,0):s,xo(l)?(r="",n!=null&&(r=n.replace(jo,"$&/")+"/"),Br(l,t,r,"",function(u){return u})):l!=null&&(gi(l)&&(l=Tu(l,r+(!l.key||o&&o.key===l.key?"":(""+l.key).replace(jo,"$&/")+"/")+n)),t.push(l)),1;if(o=0,s=s===""?".":s+":",xo(n))for(var a=0;a<n.length;a++){i=n[a];var c=s+Hs(i,a);o+=Br(i,t,r,c,l)}else if(c=Su(n),typeof c=="function")for(n=c.call(n),a=0;!(i=n.next()).done;)i=i.value,c=s+Hs(i,a++),o+=Br(i,t,r,c,l);else if(i==="object")throw t=String(n),Error("Objects are not valid as a React child (found: "+(t==="[object Object]"?"object with keys {"+Object.keys(n).join(", ")+"}":t)+"). If you meant to render a collection of children, use an array instead.");return o}function wr(n,t,r){if(n==null)return n;var s=[],l=0;return Br(n,s,"","",function(i){return t.call(r,i,l++)}),s}function Cu(n){if(n._status===-1){var t=n._result;t=t(),t.then(function(r){(n._status===0||n._status===-1)&&(n._status=1,n._result=r)},function(r){(n._status===0||n._status===-1)&&(n._status=2,n._result=r)}),n._status===-1&&(n._status=0,n._result=t)}if(n._status===1)return n._result.default;throw n._result}var ye={current:null},zr={transition:null},_u={ReactCurrentDispatcher:ye,ReactCurrentBatchConfig:zr,ReactCurrentOwner:ji};function Ga(){throw Error("act(...) is not supported in production builds of React.")}D.Children={map:wr,forEach:function(n,t,r){wr(n,function(){t.apply(this,arguments)},r)},count:function(n){var t=0;return wr(n,function(){t++}),t},toArray:function(n){return wr(n,function(t){return t})||[]},only:function(n){if(!gi(n))throw Error("React.Children.only expected to receive a single React element child.");return n}};D.Component=wt;D.Fragment=ju;D.Profiler=vu;D.PureComponent=fi;D.StrictMode=gu;D.Suspense=bu;D.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED=_u;D.act=Ga;D.cloneElement=function(n,t,r){if(n==null)throw Error("React.cloneElement(...): The argument must be a React element, but you passed "+n+".");var s=Ka({},n.props),l=n.key,i=n.ref,o=n._owner;if(t!=null){if(t.ref!==void 0&&(i=t.ref,o=ji.current),t.key!==void 0&&(l=""+t.key),n.type&&n.type.defaultProps)var a=n.type.defaultProps;for(c in t)$a.call(t,c)&&!Ha.hasOwnProperty(c)&&(s[c]=t[c]===void 0&&a!==void 0?a[c]:t[c])}var c=arguments.length-2;if(c===1)s.children=r;else if(1<c){a=Array(c);for(var u=0;u<c;u++)a[u]=arguments[u+2];s.children=a}return{$$typeof:fr,type:n.type,key:l,ref:i,props:s,_owner:o}};D.createContext=function(n){return n={$$typeof:ku,_currentValue:n,_currentValue2:n,_threadCount:0,Provider:null,Consumer:null,_defaultValue:null,_globalName:null},n.Provider={$$typeof:yu,_context:n},n.Consumer=n};D.createElement=qa;D.createFactory=function(n){var t=qa.bind(null,n);return t.type=n,t};D.createRef=function(){return{current:null}};D.forwardRef=function(n){return{$$typeof:Nu,render:n}};D.isValidElement=gi;D.lazy=function(n){return{$$typeof:Eu,_payload:{_status:-1,_result:n},_init:Cu}};D.memo=function(n,t){return{$$typeof:wu,type:n,compare:t===void 0?null:t}};D.startTransition=function(n){var t=zr.transition;zr.transition={};try{n()}finally{zr.transition=t}};D.unstable_act=Ga;D.useCallback=function(n,t){return ye.current.useCallback(n,t)};D.useContext=function(n){return ye.current.useContext(n)};D.useDebugValue=function(){};D.useDeferredValue=function(n){return ye.current.useDeferredValue(n)};D.useEffect=function(n,t){return ye.current.useEffect(n,t)};D.useId=function(){return ye.current.useId()};D.useImperativeHandle=function(n,t,r){return ye.current.useImperativeHandle(n,t,r)};D.useInsertionEffect=function(n,t){return ye.current.useInsertionEffect(n,t)};D.useLayoutEffect=function(n,t){return ye.current.useLayoutEffect(n,t)};D.useMemo=function(n,t){return ye.current.useMemo(n,t)};D.useReducer=function(n,t,r){return ye.current.useReducer(n,t,r)};D.useRef=function(n){return ye.current.useRef(n)};D.useState=function(n){return ye.current.useState(n)};D.useSyncExternalStore=function(n,t,r){return ye.current.useSyncExternalStore(n,t,r)};D.useTransition=function(){return ye.current.useTransition()};D.version="18.3.1";Da.exports=D;var y=Da.exports;const Wa=La(y),Pu=fu({__proto__:null,default:Wa},[y]);/**
 * @license React
 * react-jsx-runtime.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var Au=y,Iu=Symbol.for("react.element"),Ou=Symbol.for("react.fragment"),Mu=Object.prototype.hasOwnProperty,Lu=Au.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED.ReactCurrentOwner,Fu={key:!0,ref:!0,__self:!0,__source:!0};function Va(n,t,r){var s,l={},i=null,o=null;r!==void 0&&(i=""+r),t.key!==void 0&&(i=""+t.key),t.ref!==void 0&&(o=t.ref);for(s in t)Mu.call(t,s)&&!Fu.hasOwnProperty(s)&&(l[s]=t[s]);if(n&&n.defaultProps)for(s in t=n.defaultProps,t)l[s]===void 0&&(l[s]=t[s]);return{$$typeof:Iu,type:n,key:i,ref:o,props:l,_owner:Lu.current}}ws.Fragment=Ou;ws.jsx=Va;ws.jsxs=Va;Fa.exports=ws;var e=Fa.exports,gl={},Ya={exports:{}},Pe={},Qa={exports:{}},Xa={};/**
 * @license React
 * scheduler.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */(function(n){function t(R,O){var M=R.length;R.push(O);e:for(;0<M;){var B=M-1>>>1,F=R[B];if(0<l(F,O))R[B]=O,R[M]=F,M=B;else break e}}function r(R){return R.length===0?null:R[0]}function s(R){if(R.length===0)return null;var O=R[0],M=R.pop();if(M!==O){R[0]=M;e:for(var B=0,F=R.length,ne=F>>>1;B<ne;){var se=2*(B+1)-1,Oe=R[se],je=se+1,Me=R[je];if(0>l(Oe,M))je<F&&0>l(Me,Oe)?(R[B]=Me,R[je]=M,B=je):(R[B]=Oe,R[se]=M,B=se);else if(je<F&&0>l(Me,M))R[B]=Me,R[je]=M,B=je;else break e}}return O}function l(R,O){var M=R.sortIndex-O.sortIndex;return M!==0?M:R.id-O.id}if(typeof performance=="object"&&typeof performance.now=="function"){var i=performance;n.unstable_now=function(){return i.now()}}else{var o=Date,a=o.now();n.unstable_now=function(){return o.now()-a}}var c=[],u=[],x=1,m=null,f=3,k=!1,N=!1,g=!1,w=typeof setTimeout=="function"?setTimeout:null,p=typeof clearTimeout=="function"?clearTimeout:null,d=typeof setImmediate<"u"?setImmediate:null;typeof navigator<"u"&&navigator.scheduling!==void 0&&navigator.scheduling.isInputPending!==void 0&&navigator.scheduling.isInputPending.bind(navigator.scheduling);function h(R){for(var O=r(u);O!==null;){if(O.callback===null)s(u);else if(O.startTime<=R)s(u),O.sortIndex=O.expirationTime,t(c,O);else break;O=r(u)}}function j(R){if(g=!1,h(R),!N)if(r(c)!==null)N=!0,L(v);else{var O=r(u);O!==null&&G(j,O.startTime-R)}}function v(R,O){N=!1,g&&(g=!1,p(S),S=-1),k=!0;var M=f;try{for(h(O),m=r(c);m!==null&&(!(m.expirationTime>O)||R&&!U());){var B=m.callback;if(typeof B=="function"){m.callback=null,f=m.priorityLevel;var F=B(m.expirationTime<=O);O=n.unstable_now(),typeof F=="function"?m.callback=F:m===r(c)&&s(c),h(O)}else s(c);m=r(c)}if(m!==null)var ne=!0;else{var se=r(u);se!==null&&G(j,se.startTime-O),ne=!1}return ne}finally{m=null,f=M,k=!1}}var b=!1,E=null,S=-1,C=5,_=-1;function U(){return!(n.unstable_now()-_<C)}function A(){if(E!==null){var R=n.unstable_now();_=R;var O=!0;try{O=E(!0,R)}finally{O?z():(b=!1,E=null)}}else b=!1}var z;if(typeof d=="function")z=function(){d(A)};else if(typeof MessageChannel<"u"){var Ie=new MessageChannel,I=Ie.port2;Ie.port1.onmessage=A,z=function(){I.postMessage(null)}}else z=function(){w(A,0)};function L(R){E=R,b||(b=!0,z())}function G(R,O){S=w(function(){R(n.unstable_now())},O)}n.unstable_IdlePriority=5,n.unstable_ImmediatePriority=1,n.unstable_LowPriority=4,n.unstable_NormalPriority=3,n.unstable_Profiling=null,n.unstable_UserBlockingPriority=2,n.unstable_cancelCallback=function(R){R.callback=null},n.unstable_continueExecution=function(){N||k||(N=!0,L(v))},n.unstable_forceFrameRate=function(R){0>R||125<R?console.error("forceFrameRate takes a positive int between 0 and 125, forcing frame rates higher than 125 fps is not supported"):C=0<R?Math.floor(1e3/R):5},n.unstable_getCurrentPriorityLevel=function(){return f},n.unstable_getFirstCallbackNode=function(){return r(c)},n.unstable_next=function(R){switch(f){case 1:case 2:case 3:var O=3;break;default:O=f}var M=f;f=O;try{return R()}finally{f=M}},n.unstable_pauseExecution=function(){},n.unstable_requestPaint=function(){},n.unstable_runWithPriority=function(R,O){switch(R){case 1:case 2:case 3:case 4:case 5:break;default:R=3}var M=f;f=R;try{return O()}finally{f=M}},n.unstable_scheduleCallback=function(R,O,M){var B=n.unstable_now();switch(typeof M=="object"&&M!==null?(M=M.delay,M=typeof M=="number"&&0<M?B+M:B):M=B,R){case 1:var F=-1;break;case 2:F=250;break;case 5:F=1073741823;break;case 4:F=1e4;break;default:F=5e3}return F=M+F,R={id:x++,callback:O,priorityLevel:R,startTime:M,expirationTime:F,sortIndex:-1},M>B?(R.sortIndex=M,t(u,R),r(c)===null&&R===r(u)&&(g?(p(S),S=-1):g=!0,G(j,M-B))):(R.sortIndex=F,t(c,R),N||k||(N=!0,L(v))),R},n.unstable_shouldYield=U,n.unstable_wrapCallback=function(R){var O=f;return function(){var M=f;f=O;try{return R.apply(this,arguments)}finally{f=M}}}})(Xa);Qa.exports=Xa;var Du=Qa.exports;/**
 * @license React
 * react-dom.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var Uu=y,_e=Du;function T(n){for(var t="https://reactjs.org/docs/error-decoder.html?invariant="+n,r=1;r<arguments.length;r++)t+="&args[]="+encodeURIComponent(arguments[r]);return"Minified React error #"+n+"; visit "+t+" for the full message or use the non-minified dev environment for full errors and additional helpful warnings."}var Za=new Set,Qt={};function Wn(n,t){jt(n,t),jt(n+"Capture",t)}function jt(n,t){for(Qt[n]=t,n=0;n<t.length;n++)Za.add(t[n])}var ln=!(typeof window>"u"||typeof window.document>"u"||typeof window.document.createElement>"u"),vl=Object.prototype.hasOwnProperty,Ku=/^[:A-Z_a-z\u00C0-\u00D6\u00D8-\u00F6\u00F8-\u02FF\u0370-\u037D\u037F-\u1FFF\u200C-\u200D\u2070-\u218F\u2C00-\u2FEF\u3001-\uD7FF\uF900-\uFDCF\uFDF0-\uFFFD][:A-Z_a-z\u00C0-\u00D6\u00D8-\u00F6\u00F8-\u02FF\u0370-\u037D\u037F-\u1FFF\u200C-\u200D\u2070-\u218F\u2C00-\u2FEF\u3001-\uD7FF\uF900-\uFDCF\uFDF0-\uFFFD\-.0-9\u00B7\u0300-\u036F\u203F-\u2040]*$/,go={},vo={};function Bu(n){return vl.call(vo,n)?!0:vl.call(go,n)?!1:Ku.test(n)?vo[n]=!0:(go[n]=!0,!1)}function zu(n,t,r,s){if(r!==null&&r.type===0)return!1;switch(typeof t){case"function":case"symbol":return!0;case"boolean":return s?!1:r!==null?!r.acceptsBooleans:(n=n.toLowerCase().slice(0,5),n!=="data-"&&n!=="aria-");default:return!1}}function $u(n,t,r,s){if(t===null||typeof t>"u"||zu(n,t,r,s))return!0;if(s)return!1;if(r!==null)switch(r.type){case 3:return!t;case 4:return t===!1;case 5:return isNaN(t);case 6:return isNaN(t)||1>t}return!1}function ke(n,t,r,s,l,i,o){this.acceptsBooleans=t===2||t===3||t===4,this.attributeName=s,this.attributeNamespace=l,this.mustUseProperty=r,this.propertyName=n,this.type=t,this.sanitizeURL=i,this.removeEmptyString=o}var ue={};"children dangerouslySetInnerHTML defaultValue defaultChecked innerHTML suppressContentEditableWarning suppressHydrationWarning style".split(" ").forEach(function(n){ue[n]=new ke(n,0,!1,n,null,!1,!1)});[["acceptCharset","accept-charset"],["className","class"],["htmlFor","for"],["httpEquiv","http-equiv"]].forEach(function(n){var t=n[0];ue[t]=new ke(t,1,!1,n[1],null,!1,!1)});["contentEditable","draggable","spellCheck","value"].forEach(function(n){ue[n]=new ke(n,2,!1,n.toLowerCase(),null,!1,!1)});["autoReverse","externalResourcesRequired","focusable","preserveAlpha"].forEach(function(n){ue[n]=new ke(n,2,!1,n,null,!1,!1)});"allowFullScreen async autoFocus autoPlay controls default defer disabled disablePictureInPicture disableRemotePlayback formNoValidate hidden loop noModule noValidate open playsInline readOnly required reversed scoped seamless itemScope".split(" ").forEach(function(n){ue[n]=new ke(n,3,!1,n.toLowerCase(),null,!1,!1)});["checked","multiple","muted","selected"].forEach(function(n){ue[n]=new ke(n,3,!0,n,null,!1,!1)});["capture","download"].forEach(function(n){ue[n]=new ke(n,4,!1,n,null,!1,!1)});["cols","rows","size","span"].forEach(function(n){ue[n]=new ke(n,6,!1,n,null,!1,!1)});["rowSpan","start"].forEach(function(n){ue[n]=new ke(n,5,!1,n.toLowerCase(),null,!1,!1)});var vi=/[\-:]([a-z])/g;function yi(n){return n[1].toUpperCase()}"accent-height alignment-baseline arabic-form baseline-shift cap-height clip-path clip-rule color-interpolation color-interpolation-filters color-profile color-rendering dominant-baseline enable-background fill-opacity fill-rule flood-color flood-opacity font-family font-size font-size-adjust font-stretch font-style font-variant font-weight glyph-name glyph-orientation-horizontal glyph-orientation-vertical horiz-adv-x horiz-origin-x image-rendering letter-spacing lighting-color marker-end marker-mid marker-start overline-position overline-thickness paint-order panose-1 pointer-events rendering-intent shape-rendering stop-color stop-opacity strikethrough-position strikethrough-thickness stroke-dasharray stroke-dashoffset stroke-linecap stroke-linejoin stroke-miterlimit stroke-opacity stroke-width text-anchor text-decoration text-rendering underline-position underline-thickness unicode-bidi unicode-range units-per-em v-alphabetic v-hanging v-ideographic v-mathematical vector-effect vert-adv-y vert-origin-x vert-origin-y word-spacing writing-mode xmlns:xlink x-height".split(" ").forEach(function(n){var t=n.replace(vi,yi);ue[t]=new ke(t,1,!1,n,null,!1,!1)});"xlink:actuate xlink:arcrole xlink:role xlink:show xlink:title xlink:type".split(" ").forEach(function(n){var t=n.replace(vi,yi);ue[t]=new ke(t,1,!1,n,"http://www.w3.org/1999/xlink",!1,!1)});["xml:base","xml:lang","xml:space"].forEach(function(n){var t=n.replace(vi,yi);ue[t]=new ke(t,1,!1,n,"http://www.w3.org/XML/1998/namespace",!1,!1)});["tabIndex","crossOrigin"].forEach(function(n){ue[n]=new ke(n,1,!1,n.toLowerCase(),null,!1,!1)});ue.xlinkHref=new ke("xlinkHref",1,!1,"xlink:href","http://www.w3.org/1999/xlink",!0,!1);["src","href","action","formAction"].forEach(function(n){ue[n]=new ke(n,1,!1,n.toLowerCase(),null,!0,!0)});function ki(n,t,r,s){var l=ue.hasOwnProperty(t)?ue[t]:null;(l!==null?l.type!==0:s||!(2<t.length)||t[0]!=="o"&&t[0]!=="O"||t[1]!=="n"&&t[1]!=="N")&&($u(t,r,l,s)&&(r=null),s||l===null?Bu(t)&&(r===null?n.removeAttribute(t):n.setAttribute(t,""+r)):l.mustUseProperty?n[l.propertyName]=r===null?l.type===3?!1:"":r:(t=l.attributeName,s=l.attributeNamespace,r===null?n.removeAttribute(t):(l=l.type,r=l===3||l===4&&r===!0?"":""+r,s?n.setAttributeNS(s,t,r):n.setAttribute(t,r))))}var dn=Uu.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED,Er=Symbol.for("react.element"),Jn=Symbol.for("react.portal"),et=Symbol.for("react.fragment"),Ni=Symbol.for("react.strict_mode"),yl=Symbol.for("react.profiler"),Ja=Symbol.for("react.provider"),ec=Symbol.for("react.context"),bi=Symbol.for("react.forward_ref"),kl=Symbol.for("react.suspense"),Nl=Symbol.for("react.suspense_list"),wi=Symbol.for("react.memo"),mn=Symbol.for("react.lazy"),nc=Symbol.for("react.offscreen"),yo=Symbol.iterator;function Ct(n){return n===null||typeof n!="object"?null:(n=yo&&n[yo]||n["@@iterator"],typeof n=="function"?n:null)}var Z=Object.assign,qs;function Ft(n){if(qs===void 0)try{throw Error()}catch(r){var t=r.stack.trim().match(/\n( *(at )?)/);qs=t&&t[1]||""}return`
`+qs+n}var Gs=!1;function Ws(n,t){if(!n||Gs)return"";Gs=!0;var r=Error.prepareStackTrace;Error.prepareStackTrace=void 0;try{if(t)if(t=function(){throw Error()},Object.defineProperty(t.prototype,"props",{set:function(){throw Error()}}),typeof Reflect=="object"&&Reflect.construct){try{Reflect.construct(t,[])}catch(u){var s=u}Reflect.construct(n,[],t)}else{try{t.call()}catch(u){s=u}n.call(t.prototype)}else{try{throw Error()}catch(u){s=u}n()}}catch(u){if(u&&s&&typeof u.stack=="string"){for(var l=u.stack.split(`
`),i=s.stack.split(`
`),o=l.length-1,a=i.length-1;1<=o&&0<=a&&l[o]!==i[a];)a--;for(;1<=o&&0<=a;o--,a--)if(l[o]!==i[a]){if(o!==1||a!==1)do if(o--,a--,0>a||l[o]!==i[a]){var c=`
`+l[o].replace(" at new "," at ");return n.displayName&&c.includes("<anonymous>")&&(c=c.replace("<anonymous>",n.displayName)),c}while(1<=o&&0<=a);break}}}finally{Gs=!1,Error.prepareStackTrace=r}return(n=n?n.displayName||n.name:"")?Ft(n):""}function Hu(n){switch(n.tag){case 5:return Ft(n.type);case 16:return Ft("Lazy");case 13:return Ft("Suspense");case 19:return Ft("SuspenseList");case 0:case 2:case 15:return n=Ws(n.type,!1),n;case 11:return n=Ws(n.type.render,!1),n;case 1:return n=Ws(n.type,!0),n;default:return""}}function bl(n){if(n==null)return null;if(typeof n=="function")return n.displayName||n.name||null;if(typeof n=="string")return n;switch(n){case et:return"Fragment";case Jn:return"Portal";case yl:return"Profiler";case Ni:return"StrictMode";case kl:return"Suspense";case Nl:return"SuspenseList"}if(typeof n=="object")switch(n.$$typeof){case ec:return(n.displayName||"Context")+".Consumer";case Ja:return(n._context.displayName||"Context")+".Provider";case bi:var t=n.render;return n=n.displayName,n||(n=t.displayName||t.name||"",n=n!==""?"ForwardRef("+n+")":"ForwardRef"),n;case wi:return t=n.displayName||null,t!==null?t:bl(n.type)||"Memo";case mn:t=n._payload,n=n._init;try{return bl(n(t))}catch{}}return null}function qu(n){var t=n.type;switch(n.tag){case 24:return"Cache";case 9:return(t.displayName||"Context")+".Consumer";case 10:return(t._context.displayName||"Context")+".Provider";case 18:return"DehydratedFragment";case 11:return n=t.render,n=n.displayName||n.name||"",t.displayName||(n!==""?"ForwardRef("+n+")":"ForwardRef");case 7:return"Fragment";case 5:return t;case 4:return"Portal";case 3:return"Root";case 6:return"Text";case 16:return bl(t);case 8:return t===Ni?"StrictMode":"Mode";case 22:return"Offscreen";case 12:return"Profiler";case 21:return"Scope";case 13:return"Suspense";case 19:return"SuspenseList";case 25:return"TracingMarker";case 1:case 0:case 17:case 2:case 14:case 15:if(typeof t=="function")return t.displayName||t.name||null;if(typeof t=="string")return t}return null}function Cn(n){switch(typeof n){case"boolean":case"number":case"string":case"undefined":return n;case"object":return n;default:return""}}function tc(n){var t=n.type;return(n=n.nodeName)&&n.toLowerCase()==="input"&&(t==="checkbox"||t==="radio")}function Gu(n){var t=tc(n)?"checked":"value",r=Object.getOwnPropertyDescriptor(n.constructor.prototype,t),s=""+n[t];if(!n.hasOwnProperty(t)&&typeof r<"u"&&typeof r.get=="function"&&typeof r.set=="function"){var l=r.get,i=r.set;return Object.defineProperty(n,t,{configurable:!0,get:function(){return l.call(this)},set:function(o){s=""+o,i.call(this,o)}}),Object.defineProperty(n,t,{enumerable:r.enumerable}),{getValue:function(){return s},setValue:function(o){s=""+o},stopTracking:function(){n._valueTracker=null,delete n[t]}}}}function Sr(n){n._valueTracker||(n._valueTracker=Gu(n))}function rc(n){if(!n)return!1;var t=n._valueTracker;if(!t)return!0;var r=t.getValue(),s="";return n&&(s=tc(n)?n.checked?"true":"false":n.value),n=s,n!==r?(t.setValue(n),!0):!1}function Jr(n){if(n=n||(typeof document<"u"?document:void 0),typeof n>"u")return null;try{return n.activeElement||n.body}catch{return n.body}}function wl(n,t){var r=t.checked;return Z({},t,{defaultChecked:void 0,defaultValue:void 0,value:void 0,checked:r??n._wrapperState.initialChecked})}function ko(n,t){var r=t.defaultValue==null?"":t.defaultValue,s=t.checked!=null?t.checked:t.defaultChecked;r=Cn(t.value!=null?t.value:r),n._wrapperState={initialChecked:s,initialValue:r,controlled:t.type==="checkbox"||t.type==="radio"?t.checked!=null:t.value!=null}}function sc(n,t){t=t.checked,t!=null&&ki(n,"checked",t,!1)}function El(n,t){sc(n,t);var r=Cn(t.value),s=t.type;if(r!=null)s==="number"?(r===0&&n.value===""||n.value!=r)&&(n.value=""+r):n.value!==""+r&&(n.value=""+r);else if(s==="submit"||s==="reset"){n.removeAttribute("value");return}t.hasOwnProperty("value")?Sl(n,t.type,r):t.hasOwnProperty("defaultValue")&&Sl(n,t.type,Cn(t.defaultValue)),t.checked==null&&t.defaultChecked!=null&&(n.defaultChecked=!!t.defaultChecked)}function No(n,t,r){if(t.hasOwnProperty("value")||t.hasOwnProperty("defaultValue")){var s=t.type;if(!(s!=="submit"&&s!=="reset"||t.value!==void 0&&t.value!==null))return;t=""+n._wrapperState.initialValue,r||t===n.value||(n.value=t),n.defaultValue=t}r=n.name,r!==""&&(n.name=""),n.defaultChecked=!!n._wrapperState.initialChecked,r!==""&&(n.name=r)}function Sl(n,t,r){(t!=="number"||Jr(n.ownerDocument)!==n)&&(r==null?n.defaultValue=""+n._wrapperState.initialValue:n.defaultValue!==""+r&&(n.defaultValue=""+r))}var Dt=Array.isArray;function ut(n,t,r,s){if(n=n.options,t){t={};for(var l=0;l<r.length;l++)t["$"+r[l]]=!0;for(r=0;r<n.length;r++)l=t.hasOwnProperty("$"+n[r].value),n[r].selected!==l&&(n[r].selected=l),l&&s&&(n[r].defaultSelected=!0)}else{for(r=""+Cn(r),t=null,l=0;l<n.length;l++){if(n[l].value===r){n[l].selected=!0,s&&(n[l].defaultSelected=!0);return}t!==null||n[l].disabled||(t=n[l])}t!==null&&(t.selected=!0)}}function Tl(n,t){if(t.dangerouslySetInnerHTML!=null)throw Error(T(91));return Z({},t,{value:void 0,defaultValue:void 0,children:""+n._wrapperState.initialValue})}function bo(n,t){var r=t.value;if(r==null){if(r=t.children,t=t.defaultValue,r!=null){if(t!=null)throw Error(T(92));if(Dt(r)){if(1<r.length)throw Error(T(93));r=r[0]}t=r}t==null&&(t=""),r=t}n._wrapperState={initialValue:Cn(r)}}function lc(n,t){var r=Cn(t.value),s=Cn(t.defaultValue);r!=null&&(r=""+r,r!==n.value&&(n.value=r),t.defaultValue==null&&n.defaultValue!==r&&(n.defaultValue=r)),s!=null&&(n.defaultValue=""+s)}function wo(n){var t=n.textContent;t===n._wrapperState.initialValue&&t!==""&&t!==null&&(n.value=t)}function ic(n){switch(n){case"svg":return"http://www.w3.org/2000/svg";case"math":return"http://www.w3.org/1998/Math/MathML";default:return"http://www.w3.org/1999/xhtml"}}function Rl(n,t){return n==null||n==="http://www.w3.org/1999/xhtml"?ic(t):n==="http://www.w3.org/2000/svg"&&t==="foreignObject"?"http://www.w3.org/1999/xhtml":n}var Tr,oc=function(n){return typeof MSApp<"u"&&MSApp.execUnsafeLocalFunction?function(t,r,s,l){MSApp.execUnsafeLocalFunction(function(){return n(t,r,s,l)})}:n}(function(n,t){if(n.namespaceURI!=="http://www.w3.org/2000/svg"||"innerHTML"in n)n.innerHTML=t;else{for(Tr=Tr||document.createElement("div"),Tr.innerHTML="<svg>"+t.valueOf().toString()+"</svg>",t=Tr.firstChild;n.firstChild;)n.removeChild(n.firstChild);for(;t.firstChild;)n.appendChild(t.firstChild)}});function Xt(n,t){if(t){var r=n.firstChild;if(r&&r===n.lastChild&&r.nodeType===3){r.nodeValue=t;return}}n.textContent=t}var Bt={animationIterationCount:!0,aspectRatio:!0,borderImageOutset:!0,borderImageSlice:!0,borderImageWidth:!0,boxFlex:!0,boxFlexGroup:!0,boxOrdinalGroup:!0,columnCount:!0,columns:!0,flex:!0,flexGrow:!0,flexPositive:!0,flexShrink:!0,flexNegative:!0,flexOrder:!0,gridArea:!0,gridRow:!0,gridRowEnd:!0,gridRowSpan:!0,gridRowStart:!0,gridColumn:!0,gridColumnEnd:!0,gridColumnSpan:!0,gridColumnStart:!0,fontWeight:!0,lineClamp:!0,lineHeight:!0,opacity:!0,order:!0,orphans:!0,tabSize:!0,widows:!0,zIndex:!0,zoom:!0,fillOpacity:!0,floodOpacity:!0,stopOpacity:!0,strokeDasharray:!0,strokeDashoffset:!0,strokeMiterlimit:!0,strokeOpacity:!0,strokeWidth:!0},Wu=["Webkit","ms","Moz","O"];Object.keys(Bt).forEach(function(n){Wu.forEach(function(t){t=t+n.charAt(0).toUpperCase()+n.substring(1),Bt[t]=Bt[n]})});function ac(n,t,r){return t==null||typeof t=="boolean"||t===""?"":r||typeof t!="number"||t===0||Bt.hasOwnProperty(n)&&Bt[n]?(""+t).trim():t+"px"}function cc(n,t){n=n.style;for(var r in t)if(t.hasOwnProperty(r)){var s=r.indexOf("--")===0,l=ac(r,t[r],s);r==="float"&&(r="cssFloat"),s?n.setProperty(r,l):n[r]=l}}var Vu=Z({menuitem:!0},{area:!0,base:!0,br:!0,col:!0,embed:!0,hr:!0,img:!0,input:!0,keygen:!0,link:!0,meta:!0,param:!0,source:!0,track:!0,wbr:!0});function Cl(n,t){if(t){if(Vu[n]&&(t.children!=null||t.dangerouslySetInnerHTML!=null))throw Error(T(137,n));if(t.dangerouslySetInnerHTML!=null){if(t.children!=null)throw Error(T(60));if(typeof t.dangerouslySetInnerHTML!="object"||!("__html"in t.dangerouslySetInnerHTML))throw Error(T(61))}if(t.style!=null&&typeof t.style!="object")throw Error(T(62))}}function _l(n,t){if(n.indexOf("-")===-1)return typeof t.is=="string";switch(n){case"annotation-xml":case"color-profile":case"font-face":case"font-face-src":case"font-face-uri":case"font-face-format":case"font-face-name":case"missing-glyph":return!1;default:return!0}}var Pl=null;function Ei(n){return n=n.target||n.srcElement||window,n.correspondingUseElement&&(n=n.correspondingUseElement),n.nodeType===3?n.parentNode:n}var Al=null,ht=null,mt=null;function Eo(n){if(n=gr(n)){if(typeof Al!="function")throw Error(T(280));var t=n.stateNode;t&&(t=Cs(t),Al(n.stateNode,n.type,t))}}function dc(n){ht?mt?mt.push(n):mt=[n]:ht=n}function uc(){if(ht){var n=ht,t=mt;if(mt=ht=null,Eo(n),t)for(n=0;n<t.length;n++)Eo(t[n])}}function hc(n,t){return n(t)}function mc(){}var Vs=!1;function pc(n,t,r){if(Vs)return n(t,r);Vs=!0;try{return hc(n,t,r)}finally{Vs=!1,(ht!==null||mt!==null)&&(mc(),uc())}}function Zt(n,t){var r=n.stateNode;if(r===null)return null;var s=Cs(r);if(s===null)return null;r=s[t];e:switch(t){case"onClick":case"onClickCapture":case"onDoubleClick":case"onDoubleClickCapture":case"onMouseDown":case"onMouseDownCapture":case"onMouseMove":case"onMouseMoveCapture":case"onMouseUp":case"onMouseUpCapture":case"onMouseEnter":(s=!s.disabled)||(n=n.type,s=!(n==="button"||n==="input"||n==="select"||n==="textarea")),n=!s;break e;default:n=!1}if(n)return null;if(r&&typeof r!="function")throw Error(T(231,t,typeof r));return r}var Il=!1;if(ln)try{var _t={};Object.defineProperty(_t,"passive",{get:function(){Il=!0}}),window.addEventListener("test",_t,_t),window.removeEventListener("test",_t,_t)}catch{Il=!1}function Yu(n,t,r,s,l,i,o,a,c){var u=Array.prototype.slice.call(arguments,3);try{t.apply(r,u)}catch(x){this.onError(x)}}var zt=!1,es=null,ns=!1,Ol=null,Qu={onError:function(n){zt=!0,es=n}};function Xu(n,t,r,s,l,i,o,a,c){zt=!1,es=null,Yu.apply(Qu,arguments)}function Zu(n,t,r,s,l,i,o,a,c){if(Xu.apply(this,arguments),zt){if(zt){var u=es;zt=!1,es=null}else throw Error(T(198));ns||(ns=!0,Ol=u)}}function Vn(n){var t=n,r=n;if(n.alternate)for(;t.return;)t=t.return;else{n=t;do t=n,t.flags&4098&&(r=t.return),n=t.return;while(n)}return t.tag===3?r:null}function fc(n){if(n.tag===13){var t=n.memoizedState;if(t===null&&(n=n.alternate,n!==null&&(t=n.memoizedState)),t!==null)return t.dehydrated}return null}function So(n){if(Vn(n)!==n)throw Error(T(188))}function Ju(n){var t=n.alternate;if(!t){if(t=Vn(n),t===null)throw Error(T(188));return t!==n?null:n}for(var r=n,s=t;;){var l=r.return;if(l===null)break;var i=l.alternate;if(i===null){if(s=l.return,s!==null){r=s;continue}break}if(l.child===i.child){for(i=l.child;i;){if(i===r)return So(l),n;if(i===s)return So(l),t;i=i.sibling}throw Error(T(188))}if(r.return!==s.return)r=l,s=i;else{for(var o=!1,a=l.child;a;){if(a===r){o=!0,r=l,s=i;break}if(a===s){o=!0,s=l,r=i;break}a=a.sibling}if(!o){for(a=i.child;a;){if(a===r){o=!0,r=i,s=l;break}if(a===s){o=!0,s=i,r=l;break}a=a.sibling}if(!o)throw Error(T(189))}}if(r.alternate!==s)throw Error(T(190))}if(r.tag!==3)throw Error(T(188));return r.stateNode.current===r?n:t}function xc(n){return n=Ju(n),n!==null?jc(n):null}function jc(n){if(n.tag===5||n.tag===6)return n;for(n=n.child;n!==null;){var t=jc(n);if(t!==null)return t;n=n.sibling}return null}var gc=_e.unstable_scheduleCallback,To=_e.unstable_cancelCallback,eh=_e.unstable_shouldYield,nh=_e.unstable_requestPaint,ee=_e.unstable_now,th=_e.unstable_getCurrentPriorityLevel,Si=_e.unstable_ImmediatePriority,vc=_e.unstable_UserBlockingPriority,ts=_e.unstable_NormalPriority,rh=_e.unstable_LowPriority,yc=_e.unstable_IdlePriority,Es=null,Ze=null;function sh(n){if(Ze&&typeof Ze.onCommitFiberRoot=="function")try{Ze.onCommitFiberRoot(Es,n,void 0,(n.current.flags&128)===128)}catch{}}var Ge=Math.clz32?Math.clz32:oh,lh=Math.log,ih=Math.LN2;function oh(n){return n>>>=0,n===0?32:31-(lh(n)/ih|0)|0}var Rr=64,Cr=4194304;function Ut(n){switch(n&-n){case 1:return 1;case 2:return 2;case 4:return 4;case 8:return 8;case 16:return 16;case 32:return 32;case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:return n&4194240;case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:return n&130023424;case 134217728:return 134217728;case 268435456:return 268435456;case 536870912:return 536870912;case 1073741824:return 1073741824;default:return n}}function rs(n,t){var r=n.pendingLanes;if(r===0)return 0;var s=0,l=n.suspendedLanes,i=n.pingedLanes,o=r&268435455;if(o!==0){var a=o&~l;a!==0?s=Ut(a):(i&=o,i!==0&&(s=Ut(i)))}else o=r&~l,o!==0?s=Ut(o):i!==0&&(s=Ut(i));if(s===0)return 0;if(t!==0&&t!==s&&!(t&l)&&(l=s&-s,i=t&-t,l>=i||l===16&&(i&4194240)!==0))return t;if(s&4&&(s|=r&16),t=n.entangledLanes,t!==0)for(n=n.entanglements,t&=s;0<t;)r=31-Ge(t),l=1<<r,s|=n[r],t&=~l;return s}function ah(n,t){switch(n){case 1:case 2:case 4:return t+250;case 8:case 16:case 32:case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:return t+5e3;case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:return-1;case 134217728:case 268435456:case 536870912:case 1073741824:return-1;default:return-1}}function ch(n,t){for(var r=n.suspendedLanes,s=n.pingedLanes,l=n.expirationTimes,i=n.pendingLanes;0<i;){var o=31-Ge(i),a=1<<o,c=l[o];c===-1?(!(a&r)||a&s)&&(l[o]=ah(a,t)):c<=t&&(n.expiredLanes|=a),i&=~a}}function Ml(n){return n=n.pendingLanes&-1073741825,n!==0?n:n&1073741824?1073741824:0}function kc(){var n=Rr;return Rr<<=1,!(Rr&4194240)&&(Rr=64),n}function Ys(n){for(var t=[],r=0;31>r;r++)t.push(n);return t}function xr(n,t,r){n.pendingLanes|=t,t!==536870912&&(n.suspendedLanes=0,n.pingedLanes=0),n=n.eventTimes,t=31-Ge(t),n[t]=r}function dh(n,t){var r=n.pendingLanes&~t;n.pendingLanes=t,n.suspendedLanes=0,n.pingedLanes=0,n.expiredLanes&=t,n.mutableReadLanes&=t,n.entangledLanes&=t,t=n.entanglements;var s=n.eventTimes;for(n=n.expirationTimes;0<r;){var l=31-Ge(r),i=1<<l;t[l]=0,s[l]=-1,n[l]=-1,r&=~i}}function Ti(n,t){var r=n.entangledLanes|=t;for(n=n.entanglements;r;){var s=31-Ge(r),l=1<<s;l&t|n[s]&t&&(n[s]|=t),r&=~l}}var H=0;function Nc(n){return n&=-n,1<n?4<n?n&268435455?16:536870912:4:1}var bc,Ri,wc,Ec,Sc,Ll=!1,_r=[],yn=null,kn=null,Nn=null,Jt=new Map,er=new Map,fn=[],uh="mousedown mouseup touchcancel touchend touchstart auxclick dblclick pointercancel pointerdown pointerup dragend dragstart drop compositionend compositionstart keydown keypress keyup input textInput copy cut paste click change contextmenu reset submit".split(" ");function Ro(n,t){switch(n){case"focusin":case"focusout":yn=null;break;case"dragenter":case"dragleave":kn=null;break;case"mouseover":case"mouseout":Nn=null;break;case"pointerover":case"pointerout":Jt.delete(t.pointerId);break;case"gotpointercapture":case"lostpointercapture":er.delete(t.pointerId)}}function Pt(n,t,r,s,l,i){return n===null||n.nativeEvent!==i?(n={blockedOn:t,domEventName:r,eventSystemFlags:s,nativeEvent:i,targetContainers:[l]},t!==null&&(t=gr(t),t!==null&&Ri(t)),n):(n.eventSystemFlags|=s,t=n.targetContainers,l!==null&&t.indexOf(l)===-1&&t.push(l),n)}function hh(n,t,r,s,l){switch(t){case"focusin":return yn=Pt(yn,n,t,r,s,l),!0;case"dragenter":return kn=Pt(kn,n,t,r,s,l),!0;case"mouseover":return Nn=Pt(Nn,n,t,r,s,l),!0;case"pointerover":var i=l.pointerId;return Jt.set(i,Pt(Jt.get(i)||null,n,t,r,s,l)),!0;case"gotpointercapture":return i=l.pointerId,er.set(i,Pt(er.get(i)||null,n,t,r,s,l)),!0}return!1}function Tc(n){var t=Ln(n.target);if(t!==null){var r=Vn(t);if(r!==null){if(t=r.tag,t===13){if(t=fc(r),t!==null){n.blockedOn=t,Sc(n.priority,function(){wc(r)});return}}else if(t===3&&r.stateNode.current.memoizedState.isDehydrated){n.blockedOn=r.tag===3?r.stateNode.containerInfo:null;return}}}n.blockedOn=null}function $r(n){if(n.blockedOn!==null)return!1;for(var t=n.targetContainers;0<t.length;){var r=Fl(n.domEventName,n.eventSystemFlags,t[0],n.nativeEvent);if(r===null){r=n.nativeEvent;var s=new r.constructor(r.type,r);Pl=s,r.target.dispatchEvent(s),Pl=null}else return t=gr(r),t!==null&&Ri(t),n.blockedOn=r,!1;t.shift()}return!0}function Co(n,t,r){$r(n)&&r.delete(t)}function mh(){Ll=!1,yn!==null&&$r(yn)&&(yn=null),kn!==null&&$r(kn)&&(kn=null),Nn!==null&&$r(Nn)&&(Nn=null),Jt.forEach(Co),er.forEach(Co)}function At(n,t){n.blockedOn===t&&(n.blockedOn=null,Ll||(Ll=!0,_e.unstable_scheduleCallback(_e.unstable_NormalPriority,mh)))}function nr(n){function t(l){return At(l,n)}if(0<_r.length){At(_r[0],n);for(var r=1;r<_r.length;r++){var s=_r[r];s.blockedOn===n&&(s.blockedOn=null)}}for(yn!==null&&At(yn,n),kn!==null&&At(kn,n),Nn!==null&&At(Nn,n),Jt.forEach(t),er.forEach(t),r=0;r<fn.length;r++)s=fn[r],s.blockedOn===n&&(s.blockedOn=null);for(;0<fn.length&&(r=fn[0],r.blockedOn===null);)Tc(r),r.blockedOn===null&&fn.shift()}var pt=dn.ReactCurrentBatchConfig,ss=!0;function ph(n,t,r,s){var l=H,i=pt.transition;pt.transition=null;try{H=1,Ci(n,t,r,s)}finally{H=l,pt.transition=i}}function fh(n,t,r,s){var l=H,i=pt.transition;pt.transition=null;try{H=4,Ci(n,t,r,s)}finally{H=l,pt.transition=i}}function Ci(n,t,r,s){if(ss){var l=Fl(n,t,r,s);if(l===null)ll(n,t,s,ls,r),Ro(n,s);else if(hh(l,n,t,r,s))s.stopPropagation();else if(Ro(n,s),t&4&&-1<uh.indexOf(n)){for(;l!==null;){var i=gr(l);if(i!==null&&bc(i),i=Fl(n,t,r,s),i===null&&ll(n,t,s,ls,r),i===l)break;l=i}l!==null&&s.stopPropagation()}else ll(n,t,s,null,r)}}var ls=null;function Fl(n,t,r,s){if(ls=null,n=Ei(s),n=Ln(n),n!==null)if(t=Vn(n),t===null)n=null;else if(r=t.tag,r===13){if(n=fc(t),n!==null)return n;n=null}else if(r===3){if(t.stateNode.current.memoizedState.isDehydrated)return t.tag===3?t.stateNode.containerInfo:null;n=null}else t!==n&&(n=null);return ls=n,null}function Rc(n){switch(n){case"cancel":case"click":case"close":case"contextmenu":case"copy":case"cut":case"auxclick":case"dblclick":case"dragend":case"dragstart":case"drop":case"focusin":case"focusout":case"input":case"invalid":case"keydown":case"keypress":case"keyup":case"mousedown":case"mouseup":case"paste":case"pause":case"play":case"pointercancel":case"pointerdown":case"pointerup":case"ratechange":case"reset":case"resize":case"seeked":case"submit":case"touchcancel":case"touchend":case"touchstart":case"volumechange":case"change":case"selectionchange":case"textInput":case"compositionstart":case"compositionend":case"compositionupdate":case"beforeblur":case"afterblur":case"beforeinput":case"blur":case"fullscreenchange":case"focus":case"hashchange":case"popstate":case"select":case"selectstart":return 1;case"drag":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"mousemove":case"mouseout":case"mouseover":case"pointermove":case"pointerout":case"pointerover":case"scroll":case"toggle":case"touchmove":case"wheel":case"mouseenter":case"mouseleave":case"pointerenter":case"pointerleave":return 4;case"message":switch(th()){case Si:return 1;case vc:return 4;case ts:case rh:return 16;case yc:return 536870912;default:return 16}default:return 16}}var jn=null,_i=null,Hr=null;function Cc(){if(Hr)return Hr;var n,t=_i,r=t.length,s,l="value"in jn?jn.value:jn.textContent,i=l.length;for(n=0;n<r&&t[n]===l[n];n++);var o=r-n;for(s=1;s<=o&&t[r-s]===l[i-s];s++);return Hr=l.slice(n,1<s?1-s:void 0)}function qr(n){var t=n.keyCode;return"charCode"in n?(n=n.charCode,n===0&&t===13&&(n=13)):n=t,n===10&&(n=13),32<=n||n===13?n:0}function Pr(){return!0}function _o(){return!1}function Ae(n){function t(r,s,l,i,o){this._reactName=r,this._targetInst=l,this.type=s,this.nativeEvent=i,this.target=o,this.currentTarget=null;for(var a in n)n.hasOwnProperty(a)&&(r=n[a],this[a]=r?r(i):i[a]);return this.isDefaultPrevented=(i.defaultPrevented!=null?i.defaultPrevented:i.returnValue===!1)?Pr:_o,this.isPropagationStopped=_o,this}return Z(t.prototype,{preventDefault:function(){this.defaultPrevented=!0;var r=this.nativeEvent;r&&(r.preventDefault?r.preventDefault():typeof r.returnValue!="unknown"&&(r.returnValue=!1),this.isDefaultPrevented=Pr)},stopPropagation:function(){var r=this.nativeEvent;r&&(r.stopPropagation?r.stopPropagation():typeof r.cancelBubble!="unknown"&&(r.cancelBubble=!0),this.isPropagationStopped=Pr)},persist:function(){},isPersistent:Pr}),t}var Et={eventPhase:0,bubbles:0,cancelable:0,timeStamp:function(n){return n.timeStamp||Date.now()},defaultPrevented:0,isTrusted:0},Pi=Ae(Et),jr=Z({},Et,{view:0,detail:0}),xh=Ae(jr),Qs,Xs,It,Ss=Z({},jr,{screenX:0,screenY:0,clientX:0,clientY:0,pageX:0,pageY:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,getModifierState:Ai,button:0,buttons:0,relatedTarget:function(n){return n.relatedTarget===void 0?n.fromElement===n.srcElement?n.toElement:n.fromElement:n.relatedTarget},movementX:function(n){return"movementX"in n?n.movementX:(n!==It&&(It&&n.type==="mousemove"?(Qs=n.screenX-It.screenX,Xs=n.screenY-It.screenY):Xs=Qs=0,It=n),Qs)},movementY:function(n){return"movementY"in n?n.movementY:Xs}}),Po=Ae(Ss),jh=Z({},Ss,{dataTransfer:0}),gh=Ae(jh),vh=Z({},jr,{relatedTarget:0}),Zs=Ae(vh),yh=Z({},Et,{animationName:0,elapsedTime:0,pseudoElement:0}),kh=Ae(yh),Nh=Z({},Et,{clipboardData:function(n){return"clipboardData"in n?n.clipboardData:window.clipboardData}}),bh=Ae(Nh),wh=Z({},Et,{data:0}),Ao=Ae(wh),Eh={Esc:"Escape",Spacebar:" ",Left:"ArrowLeft",Up:"ArrowUp",Right:"ArrowRight",Down:"ArrowDown",Del:"Delete",Win:"OS",Menu:"ContextMenu",Apps:"ContextMenu",Scroll:"ScrollLock",MozPrintableKey:"Unidentified"},Sh={8:"Backspace",9:"Tab",12:"Clear",13:"Enter",16:"Shift",17:"Control",18:"Alt",19:"Pause",20:"CapsLock",27:"Escape",32:" ",33:"PageUp",34:"PageDown",35:"End",36:"Home",37:"ArrowLeft",38:"ArrowUp",39:"ArrowRight",40:"ArrowDown",45:"Insert",46:"Delete",112:"F1",113:"F2",114:"F3",115:"F4",116:"F5",117:"F6",118:"F7",119:"F8",120:"F9",121:"F10",122:"F11",123:"F12",144:"NumLock",145:"ScrollLock",224:"Meta"},Th={Alt:"altKey",Control:"ctrlKey",Meta:"metaKey",Shift:"shiftKey"};function Rh(n){var t=this.nativeEvent;return t.getModifierState?t.getModifierState(n):(n=Th[n])?!!t[n]:!1}function Ai(){return Rh}var Ch=Z({},jr,{key:function(n){if(n.key){var t=Eh[n.key]||n.key;if(t!=="Unidentified")return t}return n.type==="keypress"?(n=qr(n),n===13?"Enter":String.fromCharCode(n)):n.type==="keydown"||n.type==="keyup"?Sh[n.keyCode]||"Unidentified":""},code:0,location:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,repeat:0,locale:0,getModifierState:Ai,charCode:function(n){return n.type==="keypress"?qr(n):0},keyCode:function(n){return n.type==="keydown"||n.type==="keyup"?n.keyCode:0},which:function(n){return n.type==="keypress"?qr(n):n.type==="keydown"||n.type==="keyup"?n.keyCode:0}}),_h=Ae(Ch),Ph=Z({},Ss,{pointerId:0,width:0,height:0,pressure:0,tangentialPressure:0,tiltX:0,tiltY:0,twist:0,pointerType:0,isPrimary:0}),Io=Ae(Ph),Ah=Z({},jr,{touches:0,targetTouches:0,changedTouches:0,altKey:0,metaKey:0,ctrlKey:0,shiftKey:0,getModifierState:Ai}),Ih=Ae(Ah),Oh=Z({},Et,{propertyName:0,elapsedTime:0,pseudoElement:0}),Mh=Ae(Oh),Lh=Z({},Ss,{deltaX:function(n){return"deltaX"in n?n.deltaX:"wheelDeltaX"in n?-n.wheelDeltaX:0},deltaY:function(n){return"deltaY"in n?n.deltaY:"wheelDeltaY"in n?-n.wheelDeltaY:"wheelDelta"in n?-n.wheelDelta:0},deltaZ:0,deltaMode:0}),Fh=Ae(Lh),Dh=[9,13,27,32],Ii=ln&&"CompositionEvent"in window,$t=null;ln&&"documentMode"in document&&($t=document.documentMode);var Uh=ln&&"TextEvent"in window&&!$t,_c=ln&&(!Ii||$t&&8<$t&&11>=$t),Oo=" ",Mo=!1;function Pc(n,t){switch(n){case"keyup":return Dh.indexOf(t.keyCode)!==-1;case"keydown":return t.keyCode!==229;case"keypress":case"mousedown":case"focusout":return!0;default:return!1}}function Ac(n){return n=n.detail,typeof n=="object"&&"data"in n?n.data:null}var nt=!1;function Kh(n,t){switch(n){case"compositionend":return Ac(t);case"keypress":return t.which!==32?null:(Mo=!0,Oo);case"textInput":return n=t.data,n===Oo&&Mo?null:n;default:return null}}function Bh(n,t){if(nt)return n==="compositionend"||!Ii&&Pc(n,t)?(n=Cc(),Hr=_i=jn=null,nt=!1,n):null;switch(n){case"paste":return null;case"keypress":if(!(t.ctrlKey||t.altKey||t.metaKey)||t.ctrlKey&&t.altKey){if(t.char&&1<t.char.length)return t.char;if(t.which)return String.fromCharCode(t.which)}return null;case"compositionend":return _c&&t.locale!=="ko"?null:t.data;default:return null}}var zh={color:!0,date:!0,datetime:!0,"datetime-local":!0,email:!0,month:!0,number:!0,password:!0,range:!0,search:!0,tel:!0,text:!0,time:!0,url:!0,week:!0};function Lo(n){var t=n&&n.nodeName&&n.nodeName.toLowerCase();return t==="input"?!!zh[n.type]:t==="textarea"}function Ic(n,t,r,s){dc(s),t=is(t,"onChange"),0<t.length&&(r=new Pi("onChange","change",null,r,s),n.push({event:r,listeners:t}))}var Ht=null,tr=null;function $h(n){Hc(n,0)}function Ts(n){var t=st(n);if(rc(t))return n}function Hh(n,t){if(n==="change")return t}var Oc=!1;if(ln){var Js;if(ln){var el="oninput"in document;if(!el){var Fo=document.createElement("div");Fo.setAttribute("oninput","return;"),el=typeof Fo.oninput=="function"}Js=el}else Js=!1;Oc=Js&&(!document.documentMode||9<document.documentMode)}function Do(){Ht&&(Ht.detachEvent("onpropertychange",Mc),tr=Ht=null)}function Mc(n){if(n.propertyName==="value"&&Ts(tr)){var t=[];Ic(t,tr,n,Ei(n)),pc($h,t)}}function qh(n,t,r){n==="focusin"?(Do(),Ht=t,tr=r,Ht.attachEvent("onpropertychange",Mc)):n==="focusout"&&Do()}function Gh(n){if(n==="selectionchange"||n==="keyup"||n==="keydown")return Ts(tr)}function Wh(n,t){if(n==="click")return Ts(t)}function Vh(n,t){if(n==="input"||n==="change")return Ts(t)}function Yh(n,t){return n===t&&(n!==0||1/n===1/t)||n!==n&&t!==t}var Ve=typeof Object.is=="function"?Object.is:Yh;function rr(n,t){if(Ve(n,t))return!0;if(typeof n!="object"||n===null||typeof t!="object"||t===null)return!1;var r=Object.keys(n),s=Object.keys(t);if(r.length!==s.length)return!1;for(s=0;s<r.length;s++){var l=r[s];if(!vl.call(t,l)||!Ve(n[l],t[l]))return!1}return!0}function Uo(n){for(;n&&n.firstChild;)n=n.firstChild;return n}function Ko(n,t){var r=Uo(n);n=0;for(var s;r;){if(r.nodeType===3){if(s=n+r.textContent.length,n<=t&&s>=t)return{node:r,offset:t-n};n=s}e:{for(;r;){if(r.nextSibling){r=r.nextSibling;break e}r=r.parentNode}r=void 0}r=Uo(r)}}function Lc(n,t){return n&&t?n===t?!0:n&&n.nodeType===3?!1:t&&t.nodeType===3?Lc(n,t.parentNode):"contains"in n?n.contains(t):n.compareDocumentPosition?!!(n.compareDocumentPosition(t)&16):!1:!1}function Fc(){for(var n=window,t=Jr();t instanceof n.HTMLIFrameElement;){try{var r=typeof t.contentWindow.location.href=="string"}catch{r=!1}if(r)n=t.contentWindow;else break;t=Jr(n.document)}return t}function Oi(n){var t=n&&n.nodeName&&n.nodeName.toLowerCase();return t&&(t==="input"&&(n.type==="text"||n.type==="search"||n.type==="tel"||n.type==="url"||n.type==="password")||t==="textarea"||n.contentEditable==="true")}function Qh(n){var t=Fc(),r=n.focusedElem,s=n.selectionRange;if(t!==r&&r&&r.ownerDocument&&Lc(r.ownerDocument.documentElement,r)){if(s!==null&&Oi(r)){if(t=s.start,n=s.end,n===void 0&&(n=t),"selectionStart"in r)r.selectionStart=t,r.selectionEnd=Math.min(n,r.value.length);else if(n=(t=r.ownerDocument||document)&&t.defaultView||window,n.getSelection){n=n.getSelection();var l=r.textContent.length,i=Math.min(s.start,l);s=s.end===void 0?i:Math.min(s.end,l),!n.extend&&i>s&&(l=s,s=i,i=l),l=Ko(r,i);var o=Ko(r,s);l&&o&&(n.rangeCount!==1||n.anchorNode!==l.node||n.anchorOffset!==l.offset||n.focusNode!==o.node||n.focusOffset!==o.offset)&&(t=t.createRange(),t.setStart(l.node,l.offset),n.removeAllRanges(),i>s?(n.addRange(t),n.extend(o.node,o.offset)):(t.setEnd(o.node,o.offset),n.addRange(t)))}}for(t=[],n=r;n=n.parentNode;)n.nodeType===1&&t.push({element:n,left:n.scrollLeft,top:n.scrollTop});for(typeof r.focus=="function"&&r.focus(),r=0;r<t.length;r++)n=t[r],n.element.scrollLeft=n.left,n.element.scrollTop=n.top}}var Xh=ln&&"documentMode"in document&&11>=document.documentMode,tt=null,Dl=null,qt=null,Ul=!1;function Bo(n,t,r){var s=r.window===r?r.document:r.nodeType===9?r:r.ownerDocument;Ul||tt==null||tt!==Jr(s)||(s=tt,"selectionStart"in s&&Oi(s)?s={start:s.selectionStart,end:s.selectionEnd}:(s=(s.ownerDocument&&s.ownerDocument.defaultView||window).getSelection(),s={anchorNode:s.anchorNode,anchorOffset:s.anchorOffset,focusNode:s.focusNode,focusOffset:s.focusOffset}),qt&&rr(qt,s)||(qt=s,s=is(Dl,"onSelect"),0<s.length&&(t=new Pi("onSelect","select",null,t,r),n.push({event:t,listeners:s}),t.target=tt)))}function Ar(n,t){var r={};return r[n.toLowerCase()]=t.toLowerCase(),r["Webkit"+n]="webkit"+t,r["Moz"+n]="moz"+t,r}var rt={animationend:Ar("Animation","AnimationEnd"),animationiteration:Ar("Animation","AnimationIteration"),animationstart:Ar("Animation","AnimationStart"),transitionend:Ar("Transition","TransitionEnd")},nl={},Dc={};ln&&(Dc=document.createElement("div").style,"AnimationEvent"in window||(delete rt.animationend.animation,delete rt.animationiteration.animation,delete rt.animationstart.animation),"TransitionEvent"in window||delete rt.transitionend.transition);function Rs(n){if(nl[n])return nl[n];if(!rt[n])return n;var t=rt[n],r;for(r in t)if(t.hasOwnProperty(r)&&r in Dc)return nl[n]=t[r];return n}var Uc=Rs("animationend"),Kc=Rs("animationiteration"),Bc=Rs("animationstart"),zc=Rs("transitionend"),$c=new Map,zo="abort auxClick cancel canPlay canPlayThrough click close contextMenu copy cut drag dragEnd dragEnter dragExit dragLeave dragOver dragStart drop durationChange emptied encrypted ended error gotPointerCapture input invalid keyDown keyPress keyUp load loadedData loadedMetadata loadStart lostPointerCapture mouseDown mouseMove mouseOut mouseOver mouseUp paste pause play playing pointerCancel pointerDown pointerMove pointerOut pointerOver pointerUp progress rateChange reset resize seeked seeking stalled submit suspend timeUpdate touchCancel touchEnd touchStart volumeChange scroll toggle touchMove waiting wheel".split(" ");function Pn(n,t){$c.set(n,t),Wn(t,[n])}for(var tl=0;tl<zo.length;tl++){var rl=zo[tl],Zh=rl.toLowerCase(),Jh=rl[0].toUpperCase()+rl.slice(1);Pn(Zh,"on"+Jh)}Pn(Uc,"onAnimationEnd");Pn(Kc,"onAnimationIteration");Pn(Bc,"onAnimationStart");Pn("dblclick","onDoubleClick");Pn("focusin","onFocus");Pn("focusout","onBlur");Pn(zc,"onTransitionEnd");jt("onMouseEnter",["mouseout","mouseover"]);jt("onMouseLeave",["mouseout","mouseover"]);jt("onPointerEnter",["pointerout","pointerover"]);jt("onPointerLeave",["pointerout","pointerover"]);Wn("onChange","change click focusin focusout input keydown keyup selectionchange".split(" "));Wn("onSelect","focusout contextmenu dragend focusin keydown keyup mousedown mouseup selectionchange".split(" "));Wn("onBeforeInput",["compositionend","keypress","textInput","paste"]);Wn("onCompositionEnd","compositionend focusout keydown keypress keyup mousedown".split(" "));Wn("onCompositionStart","compositionstart focusout keydown keypress keyup mousedown".split(" "));Wn("onCompositionUpdate","compositionupdate focusout keydown keypress keyup mousedown".split(" "));var Kt="abort canplay canplaythrough durationchange emptied encrypted ended error loadeddata loadedmetadata loadstart pause play playing progress ratechange resize seeked seeking stalled suspend timeupdate volumechange waiting".split(" "),em=new Set("cancel close invalid load scroll toggle".split(" ").concat(Kt));function $o(n,t,r){var s=n.type||"unknown-event";n.currentTarget=r,Zu(s,t,void 0,n),n.currentTarget=null}function Hc(n,t){t=(t&4)!==0;for(var r=0;r<n.length;r++){var s=n[r],l=s.event;s=s.listeners;e:{var i=void 0;if(t)for(var o=s.length-1;0<=o;o--){var a=s[o],c=a.instance,u=a.currentTarget;if(a=a.listener,c!==i&&l.isPropagationStopped())break e;$o(l,a,u),i=c}else for(o=0;o<s.length;o++){if(a=s[o],c=a.instance,u=a.currentTarget,a=a.listener,c!==i&&l.isPropagationStopped())break e;$o(l,a,u),i=c}}}if(ns)throw n=Ol,ns=!1,Ol=null,n}function W(n,t){var r=t[Hl];r===void 0&&(r=t[Hl]=new Set);var s=n+"__bubble";r.has(s)||(qc(t,n,2,!1),r.add(s))}function sl(n,t,r){var s=0;t&&(s|=4),qc(r,n,s,t)}var Ir="_reactListening"+Math.random().toString(36).slice(2);function sr(n){if(!n[Ir]){n[Ir]=!0,Za.forEach(function(r){r!=="selectionchange"&&(em.has(r)||sl(r,!1,n),sl(r,!0,n))});var t=n.nodeType===9?n:n.ownerDocument;t===null||t[Ir]||(t[Ir]=!0,sl("selectionchange",!1,t))}}function qc(n,t,r,s){switch(Rc(t)){case 1:var l=ph;break;case 4:l=fh;break;default:l=Ci}r=l.bind(null,t,r,n),l=void 0,!Il||t!=="touchstart"&&t!=="touchmove"&&t!=="wheel"||(l=!0),s?l!==void 0?n.addEventListener(t,r,{capture:!0,passive:l}):n.addEventListener(t,r,!0):l!==void 0?n.addEventListener(t,r,{passive:l}):n.addEventListener(t,r,!1)}function ll(n,t,r,s,l){var i=s;if(!(t&1)&&!(t&2)&&s!==null)e:for(;;){if(s===null)return;var o=s.tag;if(o===3||o===4){var a=s.stateNode.containerInfo;if(a===l||a.nodeType===8&&a.parentNode===l)break;if(o===4)for(o=s.return;o!==null;){var c=o.tag;if((c===3||c===4)&&(c=o.stateNode.containerInfo,c===l||c.nodeType===8&&c.parentNode===l))return;o=o.return}for(;a!==null;){if(o=Ln(a),o===null)return;if(c=o.tag,c===5||c===6){s=i=o;continue e}a=a.parentNode}}s=s.return}pc(function(){var u=i,x=Ei(r),m=[];e:{var f=$c.get(n);if(f!==void 0){var k=Pi,N=n;switch(n){case"keypress":if(qr(r)===0)break e;case"keydown":case"keyup":k=_h;break;case"focusin":N="focus",k=Zs;break;case"focusout":N="blur",k=Zs;break;case"beforeblur":case"afterblur":k=Zs;break;case"click":if(r.button===2)break e;case"auxclick":case"dblclick":case"mousedown":case"mousemove":case"mouseup":case"mouseout":case"mouseover":case"contextmenu":k=Po;break;case"drag":case"dragend":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"dragstart":case"drop":k=gh;break;case"touchcancel":case"touchend":case"touchmove":case"touchstart":k=Ih;break;case Uc:case Kc:case Bc:k=kh;break;case zc:k=Mh;break;case"scroll":k=xh;break;case"wheel":k=Fh;break;case"copy":case"cut":case"paste":k=bh;break;case"gotpointercapture":case"lostpointercapture":case"pointercancel":case"pointerdown":case"pointermove":case"pointerout":case"pointerover":case"pointerup":k=Io}var g=(t&4)!==0,w=!g&&n==="scroll",p=g?f!==null?f+"Capture":null:f;g=[];for(var d=u,h;d!==null;){h=d;var j=h.stateNode;if(h.tag===5&&j!==null&&(h=j,p!==null&&(j=Zt(d,p),j!=null&&g.push(lr(d,j,h)))),w)break;d=d.return}0<g.length&&(f=new k(f,N,null,r,x),m.push({event:f,listeners:g}))}}if(!(t&7)){e:{if(f=n==="mouseover"||n==="pointerover",k=n==="mouseout"||n==="pointerout",f&&r!==Pl&&(N=r.relatedTarget||r.fromElement)&&(Ln(N)||N[on]))break e;if((k||f)&&(f=x.window===x?x:(f=x.ownerDocument)?f.defaultView||f.parentWindow:window,k?(N=r.relatedTarget||r.toElement,k=u,N=N?Ln(N):null,N!==null&&(w=Vn(N),N!==w||N.tag!==5&&N.tag!==6)&&(N=null)):(k=null,N=u),k!==N)){if(g=Po,j="onMouseLeave",p="onMouseEnter",d="mouse",(n==="pointerout"||n==="pointerover")&&(g=Io,j="onPointerLeave",p="onPointerEnter",d="pointer"),w=k==null?f:st(k),h=N==null?f:st(N),f=new g(j,d+"leave",k,r,x),f.target=w,f.relatedTarget=h,j=null,Ln(x)===u&&(g=new g(p,d+"enter",N,r,x),g.target=h,g.relatedTarget=w,j=g),w=j,k&&N)n:{for(g=k,p=N,d=0,h=g;h;h=Zn(h))d++;for(h=0,j=p;j;j=Zn(j))h++;for(;0<d-h;)g=Zn(g),d--;for(;0<h-d;)p=Zn(p),h--;for(;d--;){if(g===p||p!==null&&g===p.alternate)break n;g=Zn(g),p=Zn(p)}g=null}else g=null;k!==null&&Ho(m,f,k,g,!1),N!==null&&w!==null&&Ho(m,w,N,g,!0)}}e:{if(f=u?st(u):window,k=f.nodeName&&f.nodeName.toLowerCase(),k==="select"||k==="input"&&f.type==="file")var v=Hh;else if(Lo(f))if(Oc)v=Vh;else{v=Gh;var b=qh}else(k=f.nodeName)&&k.toLowerCase()==="input"&&(f.type==="checkbox"||f.type==="radio")&&(v=Wh);if(v&&(v=v(n,u))){Ic(m,v,r,x);break e}b&&b(n,f,u),n==="focusout"&&(b=f._wrapperState)&&b.controlled&&f.type==="number"&&Sl(f,"number",f.value)}switch(b=u?st(u):window,n){case"focusin":(Lo(b)||b.contentEditable==="true")&&(tt=b,Dl=u,qt=null);break;case"focusout":qt=Dl=tt=null;break;case"mousedown":Ul=!0;break;case"contextmenu":case"mouseup":case"dragend":Ul=!1,Bo(m,r,x);break;case"selectionchange":if(Xh)break;case"keydown":case"keyup":Bo(m,r,x)}var E;if(Ii)e:{switch(n){case"compositionstart":var S="onCompositionStart";break e;case"compositionend":S="onCompositionEnd";break e;case"compositionupdate":S="onCompositionUpdate";break e}S=void 0}else nt?Pc(n,r)&&(S="onCompositionEnd"):n==="keydown"&&r.keyCode===229&&(S="onCompositionStart");S&&(_c&&r.locale!=="ko"&&(nt||S!=="onCompositionStart"?S==="onCompositionEnd"&&nt&&(E=Cc()):(jn=x,_i="value"in jn?jn.value:jn.textContent,nt=!0)),b=is(u,S),0<b.length&&(S=new Ao(S,n,null,r,x),m.push({event:S,listeners:b}),E?S.data=E:(E=Ac(r),E!==null&&(S.data=E)))),(E=Uh?Kh(n,r):Bh(n,r))&&(u=is(u,"onBeforeInput"),0<u.length&&(x=new Ao("onBeforeInput","beforeinput",null,r,x),m.push({event:x,listeners:u}),x.data=E))}Hc(m,t)})}function lr(n,t,r){return{instance:n,listener:t,currentTarget:r}}function is(n,t){for(var r=t+"Capture",s=[];n!==null;){var l=n,i=l.stateNode;l.tag===5&&i!==null&&(l=i,i=Zt(n,r),i!=null&&s.unshift(lr(n,i,l)),i=Zt(n,t),i!=null&&s.push(lr(n,i,l))),n=n.return}return s}function Zn(n){if(n===null)return null;do n=n.return;while(n&&n.tag!==5);return n||null}function Ho(n,t,r,s,l){for(var i=t._reactName,o=[];r!==null&&r!==s;){var a=r,c=a.alternate,u=a.stateNode;if(c!==null&&c===s)break;a.tag===5&&u!==null&&(a=u,l?(c=Zt(r,i),c!=null&&o.unshift(lr(r,c,a))):l||(c=Zt(r,i),c!=null&&o.push(lr(r,c,a)))),r=r.return}o.length!==0&&n.push({event:t,listeners:o})}var nm=/\r\n?/g,tm=/\u0000|\uFFFD/g;function qo(n){return(typeof n=="string"?n:""+n).replace(nm,`
`).replace(tm,"")}function Or(n,t,r){if(t=qo(t),qo(n)!==t&&r)throw Error(T(425))}function os(){}var Kl=null,Bl=null;function zl(n,t){return n==="textarea"||n==="noscript"||typeof t.children=="string"||typeof t.children=="number"||typeof t.dangerouslySetInnerHTML=="object"&&t.dangerouslySetInnerHTML!==null&&t.dangerouslySetInnerHTML.__html!=null}var $l=typeof setTimeout=="function"?setTimeout:void 0,rm=typeof clearTimeout=="function"?clearTimeout:void 0,Go=typeof Promise=="function"?Promise:void 0,sm=typeof queueMicrotask=="function"?queueMicrotask:typeof Go<"u"?function(n){return Go.resolve(null).then(n).catch(lm)}:$l;function lm(n){setTimeout(function(){throw n})}function il(n,t){var r=t,s=0;do{var l=r.nextSibling;if(n.removeChild(r),l&&l.nodeType===8)if(r=l.data,r==="/$"){if(s===0){n.removeChild(l),nr(t);return}s--}else r!=="$"&&r!=="$?"&&r!=="$!"||s++;r=l}while(r);nr(t)}function bn(n){for(;n!=null;n=n.nextSibling){var t=n.nodeType;if(t===1||t===3)break;if(t===8){if(t=n.data,t==="$"||t==="$!"||t==="$?")break;if(t==="/$")return null}}return n}function Wo(n){n=n.previousSibling;for(var t=0;n;){if(n.nodeType===8){var r=n.data;if(r==="$"||r==="$!"||r==="$?"){if(t===0)return n;t--}else r==="/$"&&t++}n=n.previousSibling}return null}var St=Math.random().toString(36).slice(2),Xe="__reactFiber$"+St,ir="__reactProps$"+St,on="__reactContainer$"+St,Hl="__reactEvents$"+St,im="__reactListeners$"+St,om="__reactHandles$"+St;function Ln(n){var t=n[Xe];if(t)return t;for(var r=n.parentNode;r;){if(t=r[on]||r[Xe]){if(r=t.alternate,t.child!==null||r!==null&&r.child!==null)for(n=Wo(n);n!==null;){if(r=n[Xe])return r;n=Wo(n)}return t}n=r,r=n.parentNode}return null}function gr(n){return n=n[Xe]||n[on],!n||n.tag!==5&&n.tag!==6&&n.tag!==13&&n.tag!==3?null:n}function st(n){if(n.tag===5||n.tag===6)return n.stateNode;throw Error(T(33))}function Cs(n){return n[ir]||null}var ql=[],lt=-1;function An(n){return{current:n}}function V(n){0>lt||(n.current=ql[lt],ql[lt]=null,lt--)}function q(n,t){lt++,ql[lt]=n.current,n.current=t}var _n={},xe=An(_n),we=An(!1),zn=_n;function gt(n,t){var r=n.type.contextTypes;if(!r)return _n;var s=n.stateNode;if(s&&s.__reactInternalMemoizedUnmaskedChildContext===t)return s.__reactInternalMemoizedMaskedChildContext;var l={},i;for(i in r)l[i]=t[i];return s&&(n=n.stateNode,n.__reactInternalMemoizedUnmaskedChildContext=t,n.__reactInternalMemoizedMaskedChildContext=l),l}function Ee(n){return n=n.childContextTypes,n!=null}function as(){V(we),V(xe)}function Vo(n,t,r){if(xe.current!==_n)throw Error(T(168));q(xe,t),q(we,r)}function Gc(n,t,r){var s=n.stateNode;if(t=t.childContextTypes,typeof s.getChildContext!="function")return r;s=s.getChildContext();for(var l in s)if(!(l in t))throw Error(T(108,qu(n)||"Unknown",l));return Z({},r,s)}function cs(n){return n=(n=n.stateNode)&&n.__reactInternalMemoizedMergedChildContext||_n,zn=xe.current,q(xe,n),q(we,we.current),!0}function Yo(n,t,r){var s=n.stateNode;if(!s)throw Error(T(169));r?(n=Gc(n,t,zn),s.__reactInternalMemoizedMergedChildContext=n,V(we),V(xe),q(xe,n)):V(we),q(we,r)}var nn=null,_s=!1,ol=!1;function Wc(n){nn===null?nn=[n]:nn.push(n)}function am(n){_s=!0,Wc(n)}function In(){if(!ol&&nn!==null){ol=!0;var n=0,t=H;try{var r=nn;for(H=1;n<r.length;n++){var s=r[n];do s=s(!0);while(s!==null)}nn=null,_s=!1}catch(l){throw nn!==null&&(nn=nn.slice(n+1)),gc(Si,In),l}finally{H=t,ol=!1}}return null}var it=[],ot=0,ds=null,us=0,Le=[],Fe=0,$n=null,tn=1,rn="";function On(n,t){it[ot++]=us,it[ot++]=ds,ds=n,us=t}function Vc(n,t,r){Le[Fe++]=tn,Le[Fe++]=rn,Le[Fe++]=$n,$n=n;var s=tn;n=rn;var l=32-Ge(s)-1;s&=~(1<<l),r+=1;var i=32-Ge(t)+l;if(30<i){var o=l-l%5;i=(s&(1<<o)-1).toString(32),s>>=o,l-=o,tn=1<<32-Ge(t)+l|r<<l|s,rn=i+n}else tn=1<<i|r<<l|s,rn=n}function Mi(n){n.return!==null&&(On(n,1),Vc(n,1,0))}function Li(n){for(;n===ds;)ds=it[--ot],it[ot]=null,us=it[--ot],it[ot]=null;for(;n===$n;)$n=Le[--Fe],Le[Fe]=null,rn=Le[--Fe],Le[Fe]=null,tn=Le[--Fe],Le[Fe]=null}var Ce=null,Re=null,Y=!1,qe=null;function Yc(n,t){var r=De(5,null,null,0);r.elementType="DELETED",r.stateNode=t,r.return=n,t=n.deletions,t===null?(n.deletions=[r],n.flags|=16):t.push(r)}function Qo(n,t){switch(n.tag){case 5:var r=n.type;return t=t.nodeType!==1||r.toLowerCase()!==t.nodeName.toLowerCase()?null:t,t!==null?(n.stateNode=t,Ce=n,Re=bn(t.firstChild),!0):!1;case 6:return t=n.pendingProps===""||t.nodeType!==3?null:t,t!==null?(n.stateNode=t,Ce=n,Re=null,!0):!1;case 13:return t=t.nodeType!==8?null:t,t!==null?(r=$n!==null?{id:tn,overflow:rn}:null,n.memoizedState={dehydrated:t,treeContext:r,retryLane:1073741824},r=De(18,null,null,0),r.stateNode=t,r.return=n,n.child=r,Ce=n,Re=null,!0):!1;default:return!1}}function Gl(n){return(n.mode&1)!==0&&(n.flags&128)===0}function Wl(n){if(Y){var t=Re;if(t){var r=t;if(!Qo(n,t)){if(Gl(n))throw Error(T(418));t=bn(r.nextSibling);var s=Ce;t&&Qo(n,t)?Yc(s,r):(n.flags=n.flags&-4097|2,Y=!1,Ce=n)}}else{if(Gl(n))throw Error(T(418));n.flags=n.flags&-4097|2,Y=!1,Ce=n}}}function Xo(n){for(n=n.return;n!==null&&n.tag!==5&&n.tag!==3&&n.tag!==13;)n=n.return;Ce=n}function Mr(n){if(n!==Ce)return!1;if(!Y)return Xo(n),Y=!0,!1;var t;if((t=n.tag!==3)&&!(t=n.tag!==5)&&(t=n.type,t=t!=="head"&&t!=="body"&&!zl(n.type,n.memoizedProps)),t&&(t=Re)){if(Gl(n))throw Qc(),Error(T(418));for(;t;)Yc(n,t),t=bn(t.nextSibling)}if(Xo(n),n.tag===13){if(n=n.memoizedState,n=n!==null?n.dehydrated:null,!n)throw Error(T(317));e:{for(n=n.nextSibling,t=0;n;){if(n.nodeType===8){var r=n.data;if(r==="/$"){if(t===0){Re=bn(n.nextSibling);break e}t--}else r!=="$"&&r!=="$!"&&r!=="$?"||t++}n=n.nextSibling}Re=null}}else Re=Ce?bn(n.stateNode.nextSibling):null;return!0}function Qc(){for(var n=Re;n;)n=bn(n.nextSibling)}function vt(){Re=Ce=null,Y=!1}function Fi(n){qe===null?qe=[n]:qe.push(n)}var cm=dn.ReactCurrentBatchConfig;function Ot(n,t,r){if(n=r.ref,n!==null&&typeof n!="function"&&typeof n!="object"){if(r._owner){if(r=r._owner,r){if(r.tag!==1)throw Error(T(309));var s=r.stateNode}if(!s)throw Error(T(147,n));var l=s,i=""+n;return t!==null&&t.ref!==null&&typeof t.ref=="function"&&t.ref._stringRef===i?t.ref:(t=function(o){var a=l.refs;o===null?delete a[i]:a[i]=o},t._stringRef=i,t)}if(typeof n!="string")throw Error(T(284));if(!r._owner)throw Error(T(290,n))}return n}function Lr(n,t){throw n=Object.prototype.toString.call(t),Error(T(31,n==="[object Object]"?"object with keys {"+Object.keys(t).join(", ")+"}":n))}function Zo(n){var t=n._init;return t(n._payload)}function Xc(n){function t(p,d){if(n){var h=p.deletions;h===null?(p.deletions=[d],p.flags|=16):h.push(d)}}function r(p,d){if(!n)return null;for(;d!==null;)t(p,d),d=d.sibling;return null}function s(p,d){for(p=new Map;d!==null;)d.key!==null?p.set(d.key,d):p.set(d.index,d),d=d.sibling;return p}function l(p,d){return p=Tn(p,d),p.index=0,p.sibling=null,p}function i(p,d,h){return p.index=h,n?(h=p.alternate,h!==null?(h=h.index,h<d?(p.flags|=2,d):h):(p.flags|=2,d)):(p.flags|=1048576,d)}function o(p){return n&&p.alternate===null&&(p.flags|=2),p}function a(p,d,h,j){return d===null||d.tag!==6?(d=pl(h,p.mode,j),d.return=p,d):(d=l(d,h),d.return=p,d)}function c(p,d,h,j){var v=h.type;return v===et?x(p,d,h.props.children,j,h.key):d!==null&&(d.elementType===v||typeof v=="object"&&v!==null&&v.$$typeof===mn&&Zo(v)===d.type)?(j=l(d,h.props),j.ref=Ot(p,d,h),j.return=p,j):(j=Zr(h.type,h.key,h.props,null,p.mode,j),j.ref=Ot(p,d,h),j.return=p,j)}function u(p,d,h,j){return d===null||d.tag!==4||d.stateNode.containerInfo!==h.containerInfo||d.stateNode.implementation!==h.implementation?(d=fl(h,p.mode,j),d.return=p,d):(d=l(d,h.children||[]),d.return=p,d)}function x(p,d,h,j,v){return d===null||d.tag!==7?(d=Kn(h,p.mode,j,v),d.return=p,d):(d=l(d,h),d.return=p,d)}function m(p,d,h){if(typeof d=="string"&&d!==""||typeof d=="number")return d=pl(""+d,p.mode,h),d.return=p,d;if(typeof d=="object"&&d!==null){switch(d.$$typeof){case Er:return h=Zr(d.type,d.key,d.props,null,p.mode,h),h.ref=Ot(p,null,d),h.return=p,h;case Jn:return d=fl(d,p.mode,h),d.return=p,d;case mn:var j=d._init;return m(p,j(d._payload),h)}if(Dt(d)||Ct(d))return d=Kn(d,p.mode,h,null),d.return=p,d;Lr(p,d)}return null}function f(p,d,h,j){var v=d!==null?d.key:null;if(typeof h=="string"&&h!==""||typeof h=="number")return v!==null?null:a(p,d,""+h,j);if(typeof h=="object"&&h!==null){switch(h.$$typeof){case Er:return h.key===v?c(p,d,h,j):null;case Jn:return h.key===v?u(p,d,h,j):null;case mn:return v=h._init,f(p,d,v(h._payload),j)}if(Dt(h)||Ct(h))return v!==null?null:x(p,d,h,j,null);Lr(p,h)}return null}function k(p,d,h,j,v){if(typeof j=="string"&&j!==""||typeof j=="number")return p=p.get(h)||null,a(d,p,""+j,v);if(typeof j=="object"&&j!==null){switch(j.$$typeof){case Er:return p=p.get(j.key===null?h:j.key)||null,c(d,p,j,v);case Jn:return p=p.get(j.key===null?h:j.key)||null,u(d,p,j,v);case mn:var b=j._init;return k(p,d,h,b(j._payload),v)}if(Dt(j)||Ct(j))return p=p.get(h)||null,x(d,p,j,v,null);Lr(d,j)}return null}function N(p,d,h,j){for(var v=null,b=null,E=d,S=d=0,C=null;E!==null&&S<h.length;S++){E.index>S?(C=E,E=null):C=E.sibling;var _=f(p,E,h[S],j);if(_===null){E===null&&(E=C);break}n&&E&&_.alternate===null&&t(p,E),d=i(_,d,S),b===null?v=_:b.sibling=_,b=_,E=C}if(S===h.length)return r(p,E),Y&&On(p,S),v;if(E===null){for(;S<h.length;S++)E=m(p,h[S],j),E!==null&&(d=i(E,d,S),b===null?v=E:b.sibling=E,b=E);return Y&&On(p,S),v}for(E=s(p,E);S<h.length;S++)C=k(E,p,S,h[S],j),C!==null&&(n&&C.alternate!==null&&E.delete(C.key===null?S:C.key),d=i(C,d,S),b===null?v=C:b.sibling=C,b=C);return n&&E.forEach(function(U){return t(p,U)}),Y&&On(p,S),v}function g(p,d,h,j){var v=Ct(h);if(typeof v!="function")throw Error(T(150));if(h=v.call(h),h==null)throw Error(T(151));for(var b=v=null,E=d,S=d=0,C=null,_=h.next();E!==null&&!_.done;S++,_=h.next()){E.index>S?(C=E,E=null):C=E.sibling;var U=f(p,E,_.value,j);if(U===null){E===null&&(E=C);break}n&&E&&U.alternate===null&&t(p,E),d=i(U,d,S),b===null?v=U:b.sibling=U,b=U,E=C}if(_.done)return r(p,E),Y&&On(p,S),v;if(E===null){for(;!_.done;S++,_=h.next())_=m(p,_.value,j),_!==null&&(d=i(_,d,S),b===null?v=_:b.sibling=_,b=_);return Y&&On(p,S),v}for(E=s(p,E);!_.done;S++,_=h.next())_=k(E,p,S,_.value,j),_!==null&&(n&&_.alternate!==null&&E.delete(_.key===null?S:_.key),d=i(_,d,S),b===null?v=_:b.sibling=_,b=_);return n&&E.forEach(function(A){return t(p,A)}),Y&&On(p,S),v}function w(p,d,h,j){if(typeof h=="object"&&h!==null&&h.type===et&&h.key===null&&(h=h.props.children),typeof h=="object"&&h!==null){switch(h.$$typeof){case Er:e:{for(var v=h.key,b=d;b!==null;){if(b.key===v){if(v=h.type,v===et){if(b.tag===7){r(p,b.sibling),d=l(b,h.props.children),d.return=p,p=d;break e}}else if(b.elementType===v||typeof v=="object"&&v!==null&&v.$$typeof===mn&&Zo(v)===b.type){r(p,b.sibling),d=l(b,h.props),d.ref=Ot(p,b,h),d.return=p,p=d;break e}r(p,b);break}else t(p,b);b=b.sibling}h.type===et?(d=Kn(h.props.children,p.mode,j,h.key),d.return=p,p=d):(j=Zr(h.type,h.key,h.props,null,p.mode,j),j.ref=Ot(p,d,h),j.return=p,p=j)}return o(p);case Jn:e:{for(b=h.key;d!==null;){if(d.key===b)if(d.tag===4&&d.stateNode.containerInfo===h.containerInfo&&d.stateNode.implementation===h.implementation){r(p,d.sibling),d=l(d,h.children||[]),d.return=p,p=d;break e}else{r(p,d);break}else t(p,d);d=d.sibling}d=fl(h,p.mode,j),d.return=p,p=d}return o(p);case mn:return b=h._init,w(p,d,b(h._payload),j)}if(Dt(h))return N(p,d,h,j);if(Ct(h))return g(p,d,h,j);Lr(p,h)}return typeof h=="string"&&h!==""||typeof h=="number"?(h=""+h,d!==null&&d.tag===6?(r(p,d.sibling),d=l(d,h),d.return=p,p=d):(r(p,d),d=pl(h,p.mode,j),d.return=p,p=d),o(p)):r(p,d)}return w}var yt=Xc(!0),Zc=Xc(!1),hs=An(null),ms=null,at=null,Di=null;function Ui(){Di=at=ms=null}function Ki(n){var t=hs.current;V(hs),n._currentValue=t}function Vl(n,t,r){for(;n!==null;){var s=n.alternate;if((n.childLanes&t)!==t?(n.childLanes|=t,s!==null&&(s.childLanes|=t)):s!==null&&(s.childLanes&t)!==t&&(s.childLanes|=t),n===r)break;n=n.return}}function ft(n,t){ms=n,Di=at=null,n=n.dependencies,n!==null&&n.firstContext!==null&&(n.lanes&t&&(be=!0),n.firstContext=null)}function Ke(n){var t=n._currentValue;if(Di!==n)if(n={context:n,memoizedValue:t,next:null},at===null){if(ms===null)throw Error(T(308));at=n,ms.dependencies={lanes:0,firstContext:n}}else at=at.next=n;return t}var Fn=null;function Bi(n){Fn===null?Fn=[n]:Fn.push(n)}function Jc(n,t,r,s){var l=t.interleaved;return l===null?(r.next=r,Bi(t)):(r.next=l.next,l.next=r),t.interleaved=r,an(n,s)}function an(n,t){n.lanes|=t;var r=n.alternate;for(r!==null&&(r.lanes|=t),r=n,n=n.return;n!==null;)n.childLanes|=t,r=n.alternate,r!==null&&(r.childLanes|=t),r=n,n=n.return;return r.tag===3?r.stateNode:null}var pn=!1;function zi(n){n.updateQueue={baseState:n.memoizedState,firstBaseUpdate:null,lastBaseUpdate:null,shared:{pending:null,interleaved:null,lanes:0},effects:null}}function ed(n,t){n=n.updateQueue,t.updateQueue===n&&(t.updateQueue={baseState:n.baseState,firstBaseUpdate:n.firstBaseUpdate,lastBaseUpdate:n.lastBaseUpdate,shared:n.shared,effects:n.effects})}function sn(n,t){return{eventTime:n,lane:t,tag:0,payload:null,callback:null,next:null}}function wn(n,t,r){var s=n.updateQueue;if(s===null)return null;if(s=s.shared,K&2){var l=s.pending;return l===null?t.next=t:(t.next=l.next,l.next=t),s.pending=t,an(n,r)}return l=s.interleaved,l===null?(t.next=t,Bi(s)):(t.next=l.next,l.next=t),s.interleaved=t,an(n,r)}function Gr(n,t,r){if(t=t.updateQueue,t!==null&&(t=t.shared,(r&4194240)!==0)){var s=t.lanes;s&=n.pendingLanes,r|=s,t.lanes=r,Ti(n,r)}}function Jo(n,t){var r=n.updateQueue,s=n.alternate;if(s!==null&&(s=s.updateQueue,r===s)){var l=null,i=null;if(r=r.firstBaseUpdate,r!==null){do{var o={eventTime:r.eventTime,lane:r.lane,tag:r.tag,payload:r.payload,callback:r.callback,next:null};i===null?l=i=o:i=i.next=o,r=r.next}while(r!==null);i===null?l=i=t:i=i.next=t}else l=i=t;r={baseState:s.baseState,firstBaseUpdate:l,lastBaseUpdate:i,shared:s.shared,effects:s.effects},n.updateQueue=r;return}n=r.lastBaseUpdate,n===null?r.firstBaseUpdate=t:n.next=t,r.lastBaseUpdate=t}function ps(n,t,r,s){var l=n.updateQueue;pn=!1;var i=l.firstBaseUpdate,o=l.lastBaseUpdate,a=l.shared.pending;if(a!==null){l.shared.pending=null;var c=a,u=c.next;c.next=null,o===null?i=u:o.next=u,o=c;var x=n.alternate;x!==null&&(x=x.updateQueue,a=x.lastBaseUpdate,a!==o&&(a===null?x.firstBaseUpdate=u:a.next=u,x.lastBaseUpdate=c))}if(i!==null){var m=l.baseState;o=0,x=u=c=null,a=i;do{var f=a.lane,k=a.eventTime;if((s&f)===f){x!==null&&(x=x.next={eventTime:k,lane:0,tag:a.tag,payload:a.payload,callback:a.callback,next:null});e:{var N=n,g=a;switch(f=t,k=r,g.tag){case 1:if(N=g.payload,typeof N=="function"){m=N.call(k,m,f);break e}m=N;break e;case 3:N.flags=N.flags&-65537|128;case 0:if(N=g.payload,f=typeof N=="function"?N.call(k,m,f):N,f==null)break e;m=Z({},m,f);break e;case 2:pn=!0}}a.callback!==null&&a.lane!==0&&(n.flags|=64,f=l.effects,f===null?l.effects=[a]:f.push(a))}else k={eventTime:k,lane:f,tag:a.tag,payload:a.payload,callback:a.callback,next:null},x===null?(u=x=k,c=m):x=x.next=k,o|=f;if(a=a.next,a===null){if(a=l.shared.pending,a===null)break;f=a,a=f.next,f.next=null,l.lastBaseUpdate=f,l.shared.pending=null}}while(!0);if(x===null&&(c=m),l.baseState=c,l.firstBaseUpdate=u,l.lastBaseUpdate=x,t=l.shared.interleaved,t!==null){l=t;do o|=l.lane,l=l.next;while(l!==t)}else i===null&&(l.shared.lanes=0);qn|=o,n.lanes=o,n.memoizedState=m}}function ea(n,t,r){if(n=t.effects,t.effects=null,n!==null)for(t=0;t<n.length;t++){var s=n[t],l=s.callback;if(l!==null){if(s.callback=null,s=r,typeof l!="function")throw Error(T(191,l));l.call(s)}}}var vr={},Je=An(vr),or=An(vr),ar=An(vr);function Dn(n){if(n===vr)throw Error(T(174));return n}function $i(n,t){switch(q(ar,t),q(or,n),q(Je,vr),n=t.nodeType,n){case 9:case 11:t=(t=t.documentElement)?t.namespaceURI:Rl(null,"");break;default:n=n===8?t.parentNode:t,t=n.namespaceURI||null,n=n.tagName,t=Rl(t,n)}V(Je),q(Je,t)}function kt(){V(Je),V(or),V(ar)}function nd(n){Dn(ar.current);var t=Dn(Je.current),r=Rl(t,n.type);t!==r&&(q(or,n),q(Je,r))}function Hi(n){or.current===n&&(V(Je),V(or))}var Q=An(0);function fs(n){for(var t=n;t!==null;){if(t.tag===13){var r=t.memoizedState;if(r!==null&&(r=r.dehydrated,r===null||r.data==="$?"||r.data==="$!"))return t}else if(t.tag===19&&t.memoizedProps.revealOrder!==void 0){if(t.flags&128)return t}else if(t.child!==null){t.child.return=t,t=t.child;continue}if(t===n)break;for(;t.sibling===null;){if(t.return===null||t.return===n)return null;t=t.return}t.sibling.return=t.return,t=t.sibling}return null}var al=[];function qi(){for(var n=0;n<al.length;n++)al[n]._workInProgressVersionPrimary=null;al.length=0}var Wr=dn.ReactCurrentDispatcher,cl=dn.ReactCurrentBatchConfig,Hn=0,X=null,le=null,oe=null,xs=!1,Gt=!1,cr=0,dm=0;function he(){throw Error(T(321))}function Gi(n,t){if(t===null)return!1;for(var r=0;r<t.length&&r<n.length;r++)if(!Ve(n[r],t[r]))return!1;return!0}function Wi(n,t,r,s,l,i){if(Hn=i,X=t,t.memoizedState=null,t.updateQueue=null,t.lanes=0,Wr.current=n===null||n.memoizedState===null?pm:fm,n=r(s,l),Gt){i=0;do{if(Gt=!1,cr=0,25<=i)throw Error(T(301));i+=1,oe=le=null,t.updateQueue=null,Wr.current=xm,n=r(s,l)}while(Gt)}if(Wr.current=js,t=le!==null&&le.next!==null,Hn=0,oe=le=X=null,xs=!1,t)throw Error(T(300));return n}function Vi(){var n=cr!==0;return cr=0,n}function Qe(){var n={memoizedState:null,baseState:null,baseQueue:null,queue:null,next:null};return oe===null?X.memoizedState=oe=n:oe=oe.next=n,oe}function Be(){if(le===null){var n=X.alternate;n=n!==null?n.memoizedState:null}else n=le.next;var t=oe===null?X.memoizedState:oe.next;if(t!==null)oe=t,le=n;else{if(n===null)throw Error(T(310));le=n,n={memoizedState:le.memoizedState,baseState:le.baseState,baseQueue:le.baseQueue,queue:le.queue,next:null},oe===null?X.memoizedState=oe=n:oe=oe.next=n}return oe}function dr(n,t){return typeof t=="function"?t(n):t}function dl(n){var t=Be(),r=t.queue;if(r===null)throw Error(T(311));r.lastRenderedReducer=n;var s=le,l=s.baseQueue,i=r.pending;if(i!==null){if(l!==null){var o=l.next;l.next=i.next,i.next=o}s.baseQueue=l=i,r.pending=null}if(l!==null){i=l.next,s=s.baseState;var a=o=null,c=null,u=i;do{var x=u.lane;if((Hn&x)===x)c!==null&&(c=c.next={lane:0,action:u.action,hasEagerState:u.hasEagerState,eagerState:u.eagerState,next:null}),s=u.hasEagerState?u.eagerState:n(s,u.action);else{var m={lane:x,action:u.action,hasEagerState:u.hasEagerState,eagerState:u.eagerState,next:null};c===null?(a=c=m,o=s):c=c.next=m,X.lanes|=x,qn|=x}u=u.next}while(u!==null&&u!==i);c===null?o=s:c.next=a,Ve(s,t.memoizedState)||(be=!0),t.memoizedState=s,t.baseState=o,t.baseQueue=c,r.lastRenderedState=s}if(n=r.interleaved,n!==null){l=n;do i=l.lane,X.lanes|=i,qn|=i,l=l.next;while(l!==n)}else l===null&&(r.lanes=0);return[t.memoizedState,r.dispatch]}function ul(n){var t=Be(),r=t.queue;if(r===null)throw Error(T(311));r.lastRenderedReducer=n;var s=r.dispatch,l=r.pending,i=t.memoizedState;if(l!==null){r.pending=null;var o=l=l.next;do i=n(i,o.action),o=o.next;while(o!==l);Ve(i,t.memoizedState)||(be=!0),t.memoizedState=i,t.baseQueue===null&&(t.baseState=i),r.lastRenderedState=i}return[i,s]}function td(){}function rd(n,t){var r=X,s=Be(),l=t(),i=!Ve(s.memoizedState,l);if(i&&(s.memoizedState=l,be=!0),s=s.queue,Yi(id.bind(null,r,s,n),[n]),s.getSnapshot!==t||i||oe!==null&&oe.memoizedState.tag&1){if(r.flags|=2048,ur(9,ld.bind(null,r,s,l,t),void 0,null),ae===null)throw Error(T(349));Hn&30||sd(r,t,l)}return l}function sd(n,t,r){n.flags|=16384,n={getSnapshot:t,value:r},t=X.updateQueue,t===null?(t={lastEffect:null,stores:null},X.updateQueue=t,t.stores=[n]):(r=t.stores,r===null?t.stores=[n]:r.push(n))}function ld(n,t,r,s){t.value=r,t.getSnapshot=s,od(t)&&ad(n)}function id(n,t,r){return r(function(){od(t)&&ad(n)})}function od(n){var t=n.getSnapshot;n=n.value;try{var r=t();return!Ve(n,r)}catch{return!0}}function ad(n){var t=an(n,1);t!==null&&We(t,n,1,-1)}function na(n){var t=Qe();return typeof n=="function"&&(n=n()),t.memoizedState=t.baseState=n,n={pending:null,interleaved:null,lanes:0,dispatch:null,lastRenderedReducer:dr,lastRenderedState:n},t.queue=n,n=n.dispatch=mm.bind(null,X,n),[t.memoizedState,n]}function ur(n,t,r,s){return n={tag:n,create:t,destroy:r,deps:s,next:null},t=X.updateQueue,t===null?(t={lastEffect:null,stores:null},X.updateQueue=t,t.lastEffect=n.next=n):(r=t.lastEffect,r===null?t.lastEffect=n.next=n:(s=r.next,r.next=n,n.next=s,t.lastEffect=n)),n}function cd(){return Be().memoizedState}function Vr(n,t,r,s){var l=Qe();X.flags|=n,l.memoizedState=ur(1|t,r,void 0,s===void 0?null:s)}function Ps(n,t,r,s){var l=Be();s=s===void 0?null:s;var i=void 0;if(le!==null){var o=le.memoizedState;if(i=o.destroy,s!==null&&Gi(s,o.deps)){l.memoizedState=ur(t,r,i,s);return}}X.flags|=n,l.memoizedState=ur(1|t,r,i,s)}function ta(n,t){return Vr(8390656,8,n,t)}function Yi(n,t){return Ps(2048,8,n,t)}function dd(n,t){return Ps(4,2,n,t)}function ud(n,t){return Ps(4,4,n,t)}function hd(n,t){if(typeof t=="function")return n=n(),t(n),function(){t(null)};if(t!=null)return n=n(),t.current=n,function(){t.current=null}}function md(n,t,r){return r=r!=null?r.concat([n]):null,Ps(4,4,hd.bind(null,t,n),r)}function Qi(){}function pd(n,t){var r=Be();t=t===void 0?null:t;var s=r.memoizedState;return s!==null&&t!==null&&Gi(t,s[1])?s[0]:(r.memoizedState=[n,t],n)}function fd(n,t){var r=Be();t=t===void 0?null:t;var s=r.memoizedState;return s!==null&&t!==null&&Gi(t,s[1])?s[0]:(n=n(),r.memoizedState=[n,t],n)}function xd(n,t,r){return Hn&21?(Ve(r,t)||(r=kc(),X.lanes|=r,qn|=r,n.baseState=!0),t):(n.baseState&&(n.baseState=!1,be=!0),n.memoizedState=r)}function um(n,t){var r=H;H=r!==0&&4>r?r:4,n(!0);var s=cl.transition;cl.transition={};try{n(!1),t()}finally{H=r,cl.transition=s}}function jd(){return Be().memoizedState}function hm(n,t,r){var s=Sn(n);if(r={lane:s,action:r,hasEagerState:!1,eagerState:null,next:null},gd(n))vd(t,r);else if(r=Jc(n,t,r,s),r!==null){var l=ve();We(r,n,s,l),yd(r,t,s)}}function mm(n,t,r){var s=Sn(n),l={lane:s,action:r,hasEagerState:!1,eagerState:null,next:null};if(gd(n))vd(t,l);else{var i=n.alternate;if(n.lanes===0&&(i===null||i.lanes===0)&&(i=t.lastRenderedReducer,i!==null))try{var o=t.lastRenderedState,a=i(o,r);if(l.hasEagerState=!0,l.eagerState=a,Ve(a,o)){var c=t.interleaved;c===null?(l.next=l,Bi(t)):(l.next=c.next,c.next=l),t.interleaved=l;return}}catch{}finally{}r=Jc(n,t,l,s),r!==null&&(l=ve(),We(r,n,s,l),yd(r,t,s))}}function gd(n){var t=n.alternate;return n===X||t!==null&&t===X}function vd(n,t){Gt=xs=!0;var r=n.pending;r===null?t.next=t:(t.next=r.next,r.next=t),n.pending=t}function yd(n,t,r){if(r&4194240){var s=t.lanes;s&=n.pendingLanes,r|=s,t.lanes=r,Ti(n,r)}}var js={readContext:Ke,useCallback:he,useContext:he,useEffect:he,useImperativeHandle:he,useInsertionEffect:he,useLayoutEffect:he,useMemo:he,useReducer:he,useRef:he,useState:he,useDebugValue:he,useDeferredValue:he,useTransition:he,useMutableSource:he,useSyncExternalStore:he,useId:he,unstable_isNewReconciler:!1},pm={readContext:Ke,useCallback:function(n,t){return Qe().memoizedState=[n,t===void 0?null:t],n},useContext:Ke,useEffect:ta,useImperativeHandle:function(n,t,r){return r=r!=null?r.concat([n]):null,Vr(4194308,4,hd.bind(null,t,n),r)},useLayoutEffect:function(n,t){return Vr(4194308,4,n,t)},useInsertionEffect:function(n,t){return Vr(4,2,n,t)},useMemo:function(n,t){var r=Qe();return t=t===void 0?null:t,n=n(),r.memoizedState=[n,t],n},useReducer:function(n,t,r){var s=Qe();return t=r!==void 0?r(t):t,s.memoizedState=s.baseState=t,n={pending:null,interleaved:null,lanes:0,dispatch:null,lastRenderedReducer:n,lastRenderedState:t},s.queue=n,n=n.dispatch=hm.bind(null,X,n),[s.memoizedState,n]},useRef:function(n){var t=Qe();return n={current:n},t.memoizedState=n},useState:na,useDebugValue:Qi,useDeferredValue:function(n){return Qe().memoizedState=n},useTransition:function(){var n=na(!1),t=n[0];return n=um.bind(null,n[1]),Qe().memoizedState=n,[t,n]},useMutableSource:function(){},useSyncExternalStore:function(n,t,r){var s=X,l=Qe();if(Y){if(r===void 0)throw Error(T(407));r=r()}else{if(r=t(),ae===null)throw Error(T(349));Hn&30||sd(s,t,r)}l.memoizedState=r;var i={value:r,getSnapshot:t};return l.queue=i,ta(id.bind(null,s,i,n),[n]),s.flags|=2048,ur(9,ld.bind(null,s,i,r,t),void 0,null),r},useId:function(){var n=Qe(),t=ae.identifierPrefix;if(Y){var r=rn,s=tn;r=(s&~(1<<32-Ge(s)-1)).toString(32)+r,t=":"+t+"R"+r,r=cr++,0<r&&(t+="H"+r.toString(32)),t+=":"}else r=dm++,t=":"+t+"r"+r.toString(32)+":";return n.memoizedState=t},unstable_isNewReconciler:!1},fm={readContext:Ke,useCallback:pd,useContext:Ke,useEffect:Yi,useImperativeHandle:md,useInsertionEffect:dd,useLayoutEffect:ud,useMemo:fd,useReducer:dl,useRef:cd,useState:function(){return dl(dr)},useDebugValue:Qi,useDeferredValue:function(n){var t=Be();return xd(t,le.memoizedState,n)},useTransition:function(){var n=dl(dr)[0],t=Be().memoizedState;return[n,t]},useMutableSource:td,useSyncExternalStore:rd,useId:jd,unstable_isNewReconciler:!1},xm={readContext:Ke,useCallback:pd,useContext:Ke,useEffect:Yi,useImperativeHandle:md,useInsertionEffect:dd,useLayoutEffect:ud,useMemo:fd,useReducer:ul,useRef:cd,useState:function(){return ul(dr)},useDebugValue:Qi,useDeferredValue:function(n){var t=Be();return le===null?t.memoizedState=n:xd(t,le.memoizedState,n)},useTransition:function(){var n=ul(dr)[0],t=Be().memoizedState;return[n,t]},useMutableSource:td,useSyncExternalStore:rd,useId:jd,unstable_isNewReconciler:!1};function $e(n,t){if(n&&n.defaultProps){t=Z({},t),n=n.defaultProps;for(var r in n)t[r]===void 0&&(t[r]=n[r]);return t}return t}function Yl(n,t,r,s){t=n.memoizedState,r=r(s,t),r=r==null?t:Z({},t,r),n.memoizedState=r,n.lanes===0&&(n.updateQueue.baseState=r)}var As={isMounted:function(n){return(n=n._reactInternals)?Vn(n)===n:!1},enqueueSetState:function(n,t,r){n=n._reactInternals;var s=ve(),l=Sn(n),i=sn(s,l);i.payload=t,r!=null&&(i.callback=r),t=wn(n,i,l),t!==null&&(We(t,n,l,s),Gr(t,n,l))},enqueueReplaceState:function(n,t,r){n=n._reactInternals;var s=ve(),l=Sn(n),i=sn(s,l);i.tag=1,i.payload=t,r!=null&&(i.callback=r),t=wn(n,i,l),t!==null&&(We(t,n,l,s),Gr(t,n,l))},enqueueForceUpdate:function(n,t){n=n._reactInternals;var r=ve(),s=Sn(n),l=sn(r,s);l.tag=2,t!=null&&(l.callback=t),t=wn(n,l,s),t!==null&&(We(t,n,s,r),Gr(t,n,s))}};function ra(n,t,r,s,l,i,o){return n=n.stateNode,typeof n.shouldComponentUpdate=="function"?n.shouldComponentUpdate(s,i,o):t.prototype&&t.prototype.isPureReactComponent?!rr(r,s)||!rr(l,i):!0}function kd(n,t,r){var s=!1,l=_n,i=t.contextType;return typeof i=="object"&&i!==null?i=Ke(i):(l=Ee(t)?zn:xe.current,s=t.contextTypes,i=(s=s!=null)?gt(n,l):_n),t=new t(r,i),n.memoizedState=t.state!==null&&t.state!==void 0?t.state:null,t.updater=As,n.stateNode=t,t._reactInternals=n,s&&(n=n.stateNode,n.__reactInternalMemoizedUnmaskedChildContext=l,n.__reactInternalMemoizedMaskedChildContext=i),t}function sa(n,t,r,s){n=t.state,typeof t.componentWillReceiveProps=="function"&&t.componentWillReceiveProps(r,s),typeof t.UNSAFE_componentWillReceiveProps=="function"&&t.UNSAFE_componentWillReceiveProps(r,s),t.state!==n&&As.enqueueReplaceState(t,t.state,null)}function Ql(n,t,r,s){var l=n.stateNode;l.props=r,l.state=n.memoizedState,l.refs={},zi(n);var i=t.contextType;typeof i=="object"&&i!==null?l.context=Ke(i):(i=Ee(t)?zn:xe.current,l.context=gt(n,i)),l.state=n.memoizedState,i=t.getDerivedStateFromProps,typeof i=="function"&&(Yl(n,t,i,r),l.state=n.memoizedState),typeof t.getDerivedStateFromProps=="function"||typeof l.getSnapshotBeforeUpdate=="function"||typeof l.UNSAFE_componentWillMount!="function"&&typeof l.componentWillMount!="function"||(t=l.state,typeof l.componentWillMount=="function"&&l.componentWillMount(),typeof l.UNSAFE_componentWillMount=="function"&&l.UNSAFE_componentWillMount(),t!==l.state&&As.enqueueReplaceState(l,l.state,null),ps(n,r,l,s),l.state=n.memoizedState),typeof l.componentDidMount=="function"&&(n.flags|=4194308)}function Nt(n,t){try{var r="",s=t;do r+=Hu(s),s=s.return;while(s);var l=r}catch(i){l=`
Error generating stack: `+i.message+`
`+i.stack}return{value:n,source:t,stack:l,digest:null}}function hl(n,t,r){return{value:n,source:null,stack:r??null,digest:t??null}}function Xl(n,t){try{console.error(t.value)}catch(r){setTimeout(function(){throw r})}}var jm=typeof WeakMap=="function"?WeakMap:Map;function Nd(n,t,r){r=sn(-1,r),r.tag=3,r.payload={element:null};var s=t.value;return r.callback=function(){vs||(vs=!0,oi=s),Xl(n,t)},r}function bd(n,t,r){r=sn(-1,r),r.tag=3;var s=n.type.getDerivedStateFromError;if(typeof s=="function"){var l=t.value;r.payload=function(){return s(l)},r.callback=function(){Xl(n,t)}}var i=n.stateNode;return i!==null&&typeof i.componentDidCatch=="function"&&(r.callback=function(){Xl(n,t),typeof s!="function"&&(En===null?En=new Set([this]):En.add(this));var o=t.stack;this.componentDidCatch(t.value,{componentStack:o!==null?o:""})}),r}function la(n,t,r){var s=n.pingCache;if(s===null){s=n.pingCache=new jm;var l=new Set;s.set(t,l)}else l=s.get(t),l===void 0&&(l=new Set,s.set(t,l));l.has(r)||(l.add(r),n=Pm.bind(null,n,t,r),t.then(n,n))}function ia(n){do{var t;if((t=n.tag===13)&&(t=n.memoizedState,t=t!==null?t.dehydrated!==null:!0),t)return n;n=n.return}while(n!==null);return null}function oa(n,t,r,s,l){return n.mode&1?(n.flags|=65536,n.lanes=l,n):(n===t?n.flags|=65536:(n.flags|=128,r.flags|=131072,r.flags&=-52805,r.tag===1&&(r.alternate===null?r.tag=17:(t=sn(-1,1),t.tag=2,wn(r,t,1))),r.lanes|=1),n)}var gm=dn.ReactCurrentOwner,be=!1;function ge(n,t,r,s){t.child=n===null?Zc(t,null,r,s):yt(t,n.child,r,s)}function aa(n,t,r,s,l){r=r.render;var i=t.ref;return ft(t,l),s=Wi(n,t,r,s,i,l),r=Vi(),n!==null&&!be?(t.updateQueue=n.updateQueue,t.flags&=-2053,n.lanes&=~l,cn(n,t,l)):(Y&&r&&Mi(t),t.flags|=1,ge(n,t,s,l),t.child)}function ca(n,t,r,s,l){if(n===null){var i=r.type;return typeof i=="function"&&!so(i)&&i.defaultProps===void 0&&r.compare===null&&r.defaultProps===void 0?(t.tag=15,t.type=i,wd(n,t,i,s,l)):(n=Zr(r.type,null,s,t,t.mode,l),n.ref=t.ref,n.return=t,t.child=n)}if(i=n.child,!(n.lanes&l)){var o=i.memoizedProps;if(r=r.compare,r=r!==null?r:rr,r(o,s)&&n.ref===t.ref)return cn(n,t,l)}return t.flags|=1,n=Tn(i,s),n.ref=t.ref,n.return=t,t.child=n}function wd(n,t,r,s,l){if(n!==null){var i=n.memoizedProps;if(rr(i,s)&&n.ref===t.ref)if(be=!1,t.pendingProps=s=i,(n.lanes&l)!==0)n.flags&131072&&(be=!0);else return t.lanes=n.lanes,cn(n,t,l)}return Zl(n,t,r,s,l)}function Ed(n,t,r){var s=t.pendingProps,l=s.children,i=n!==null?n.memoizedState:null;if(s.mode==="hidden")if(!(t.mode&1))t.memoizedState={baseLanes:0,cachePool:null,transitions:null},q(dt,Te),Te|=r;else{if(!(r&1073741824))return n=i!==null?i.baseLanes|r:r,t.lanes=t.childLanes=1073741824,t.memoizedState={baseLanes:n,cachePool:null,transitions:null},t.updateQueue=null,q(dt,Te),Te|=n,null;t.memoizedState={baseLanes:0,cachePool:null,transitions:null},s=i!==null?i.baseLanes:r,q(dt,Te),Te|=s}else i!==null?(s=i.baseLanes|r,t.memoizedState=null):s=r,q(dt,Te),Te|=s;return ge(n,t,l,r),t.child}function Sd(n,t){var r=t.ref;(n===null&&r!==null||n!==null&&n.ref!==r)&&(t.flags|=512,t.flags|=2097152)}function Zl(n,t,r,s,l){var i=Ee(r)?zn:xe.current;return i=gt(t,i),ft(t,l),r=Wi(n,t,r,s,i,l),s=Vi(),n!==null&&!be?(t.updateQueue=n.updateQueue,t.flags&=-2053,n.lanes&=~l,cn(n,t,l)):(Y&&s&&Mi(t),t.flags|=1,ge(n,t,r,l),t.child)}function da(n,t,r,s,l){if(Ee(r)){var i=!0;cs(t)}else i=!1;if(ft(t,l),t.stateNode===null)Yr(n,t),kd(t,r,s),Ql(t,r,s,l),s=!0;else if(n===null){var o=t.stateNode,a=t.memoizedProps;o.props=a;var c=o.context,u=r.contextType;typeof u=="object"&&u!==null?u=Ke(u):(u=Ee(r)?zn:xe.current,u=gt(t,u));var x=r.getDerivedStateFromProps,m=typeof x=="function"||typeof o.getSnapshotBeforeUpdate=="function";m||typeof o.UNSAFE_componentWillReceiveProps!="function"&&typeof o.componentWillReceiveProps!="function"||(a!==s||c!==u)&&sa(t,o,s,u),pn=!1;var f=t.memoizedState;o.state=f,ps(t,s,o,l),c=t.memoizedState,a!==s||f!==c||we.current||pn?(typeof x=="function"&&(Yl(t,r,x,s),c=t.memoizedState),(a=pn||ra(t,r,a,s,f,c,u))?(m||typeof o.UNSAFE_componentWillMount!="function"&&typeof o.componentWillMount!="function"||(typeof o.componentWillMount=="function"&&o.componentWillMount(),typeof o.UNSAFE_componentWillMount=="function"&&o.UNSAFE_componentWillMount()),typeof o.componentDidMount=="function"&&(t.flags|=4194308)):(typeof o.componentDidMount=="function"&&(t.flags|=4194308),t.memoizedProps=s,t.memoizedState=c),o.props=s,o.state=c,o.context=u,s=a):(typeof o.componentDidMount=="function"&&(t.flags|=4194308),s=!1)}else{o=t.stateNode,ed(n,t),a=t.memoizedProps,u=t.type===t.elementType?a:$e(t.type,a),o.props=u,m=t.pendingProps,f=o.context,c=r.contextType,typeof c=="object"&&c!==null?c=Ke(c):(c=Ee(r)?zn:xe.current,c=gt(t,c));var k=r.getDerivedStateFromProps;(x=typeof k=="function"||typeof o.getSnapshotBeforeUpdate=="function")||typeof o.UNSAFE_componentWillReceiveProps!="function"&&typeof o.componentWillReceiveProps!="function"||(a!==m||f!==c)&&sa(t,o,s,c),pn=!1,f=t.memoizedState,o.state=f,ps(t,s,o,l);var N=t.memoizedState;a!==m||f!==N||we.current||pn?(typeof k=="function"&&(Yl(t,r,k,s),N=t.memoizedState),(u=pn||ra(t,r,u,s,f,N,c)||!1)?(x||typeof o.UNSAFE_componentWillUpdate!="function"&&typeof o.componentWillUpdate!="function"||(typeof o.componentWillUpdate=="function"&&o.componentWillUpdate(s,N,c),typeof o.UNSAFE_componentWillUpdate=="function"&&o.UNSAFE_componentWillUpdate(s,N,c)),typeof o.componentDidUpdate=="function"&&(t.flags|=4),typeof o.getSnapshotBeforeUpdate=="function"&&(t.flags|=1024)):(typeof o.componentDidUpdate!="function"||a===n.memoizedProps&&f===n.memoizedState||(t.flags|=4),typeof o.getSnapshotBeforeUpdate!="function"||a===n.memoizedProps&&f===n.memoizedState||(t.flags|=1024),t.memoizedProps=s,t.memoizedState=N),o.props=s,o.state=N,o.context=c,s=u):(typeof o.componentDidUpdate!="function"||a===n.memoizedProps&&f===n.memoizedState||(t.flags|=4),typeof o.getSnapshotBeforeUpdate!="function"||a===n.memoizedProps&&f===n.memoizedState||(t.flags|=1024),s=!1)}return Jl(n,t,r,s,i,l)}function Jl(n,t,r,s,l,i){Sd(n,t);var o=(t.flags&128)!==0;if(!s&&!o)return l&&Yo(t,r,!1),cn(n,t,i);s=t.stateNode,gm.current=t;var a=o&&typeof r.getDerivedStateFromError!="function"?null:s.render();return t.flags|=1,n!==null&&o?(t.child=yt(t,n.child,null,i),t.child=yt(t,null,a,i)):ge(n,t,a,i),t.memoizedState=s.state,l&&Yo(t,r,!0),t.child}function Td(n){var t=n.stateNode;t.pendingContext?Vo(n,t.pendingContext,t.pendingContext!==t.context):t.context&&Vo(n,t.context,!1),$i(n,t.containerInfo)}function ua(n,t,r,s,l){return vt(),Fi(l),t.flags|=256,ge(n,t,r,s),t.child}var ei={dehydrated:null,treeContext:null,retryLane:0};function ni(n){return{baseLanes:n,cachePool:null,transitions:null}}function Rd(n,t,r){var s=t.pendingProps,l=Q.current,i=!1,o=(t.flags&128)!==0,a;if((a=o)||(a=n!==null&&n.memoizedState===null?!1:(l&2)!==0),a?(i=!0,t.flags&=-129):(n===null||n.memoizedState!==null)&&(l|=1),q(Q,l&1),n===null)return Wl(t),n=t.memoizedState,n!==null&&(n=n.dehydrated,n!==null)?(t.mode&1?n.data==="$!"?t.lanes=8:t.lanes=1073741824:t.lanes=1,null):(o=s.children,n=s.fallback,i?(s=t.mode,i=t.child,o={mode:"hidden",children:o},!(s&1)&&i!==null?(i.childLanes=0,i.pendingProps=o):i=Ms(o,s,0,null),n=Kn(n,s,r,null),i.return=t,n.return=t,i.sibling=n,t.child=i,t.child.memoizedState=ni(r),t.memoizedState=ei,n):Xi(t,o));if(l=n.memoizedState,l!==null&&(a=l.dehydrated,a!==null))return vm(n,t,o,s,a,l,r);if(i){i=s.fallback,o=t.mode,l=n.child,a=l.sibling;var c={mode:"hidden",children:s.children};return!(o&1)&&t.child!==l?(s=t.child,s.childLanes=0,s.pendingProps=c,t.deletions=null):(s=Tn(l,c),s.subtreeFlags=l.subtreeFlags&14680064),a!==null?i=Tn(a,i):(i=Kn(i,o,r,null),i.flags|=2),i.return=t,s.return=t,s.sibling=i,t.child=s,s=i,i=t.child,o=n.child.memoizedState,o=o===null?ni(r):{baseLanes:o.baseLanes|r,cachePool:null,transitions:o.transitions},i.memoizedState=o,i.childLanes=n.childLanes&~r,t.memoizedState=ei,s}return i=n.child,n=i.sibling,s=Tn(i,{mode:"visible",children:s.children}),!(t.mode&1)&&(s.lanes=r),s.return=t,s.sibling=null,n!==null&&(r=t.deletions,r===null?(t.deletions=[n],t.flags|=16):r.push(n)),t.child=s,t.memoizedState=null,s}function Xi(n,t){return t=Ms({mode:"visible",children:t},n.mode,0,null),t.return=n,n.child=t}function Fr(n,t,r,s){return s!==null&&Fi(s),yt(t,n.child,null,r),n=Xi(t,t.pendingProps.children),n.flags|=2,t.memoizedState=null,n}function vm(n,t,r,s,l,i,o){if(r)return t.flags&256?(t.flags&=-257,s=hl(Error(T(422))),Fr(n,t,o,s)):t.memoizedState!==null?(t.child=n.child,t.flags|=128,null):(i=s.fallback,l=t.mode,s=Ms({mode:"visible",children:s.children},l,0,null),i=Kn(i,l,o,null),i.flags|=2,s.return=t,i.return=t,s.sibling=i,t.child=s,t.mode&1&&yt(t,n.child,null,o),t.child.memoizedState=ni(o),t.memoizedState=ei,i);if(!(t.mode&1))return Fr(n,t,o,null);if(l.data==="$!"){if(s=l.nextSibling&&l.nextSibling.dataset,s)var a=s.dgst;return s=a,i=Error(T(419)),s=hl(i,s,void 0),Fr(n,t,o,s)}if(a=(o&n.childLanes)!==0,be||a){if(s=ae,s!==null){switch(o&-o){case 4:l=2;break;case 16:l=8;break;case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:l=32;break;case 536870912:l=268435456;break;default:l=0}l=l&(s.suspendedLanes|o)?0:l,l!==0&&l!==i.retryLane&&(i.retryLane=l,an(n,l),We(s,n,l,-1))}return ro(),s=hl(Error(T(421))),Fr(n,t,o,s)}return l.data==="$?"?(t.flags|=128,t.child=n.child,t=Am.bind(null,n),l._reactRetry=t,null):(n=i.treeContext,Re=bn(l.nextSibling),Ce=t,Y=!0,qe=null,n!==null&&(Le[Fe++]=tn,Le[Fe++]=rn,Le[Fe++]=$n,tn=n.id,rn=n.overflow,$n=t),t=Xi(t,s.children),t.flags|=4096,t)}function ha(n,t,r){n.lanes|=t;var s=n.alternate;s!==null&&(s.lanes|=t),Vl(n.return,t,r)}function ml(n,t,r,s,l){var i=n.memoizedState;i===null?n.memoizedState={isBackwards:t,rendering:null,renderingStartTime:0,last:s,tail:r,tailMode:l}:(i.isBackwards=t,i.rendering=null,i.renderingStartTime=0,i.last=s,i.tail=r,i.tailMode=l)}function Cd(n,t,r){var s=t.pendingProps,l=s.revealOrder,i=s.tail;if(ge(n,t,s.children,r),s=Q.current,s&2)s=s&1|2,t.flags|=128;else{if(n!==null&&n.flags&128)e:for(n=t.child;n!==null;){if(n.tag===13)n.memoizedState!==null&&ha(n,r,t);else if(n.tag===19)ha(n,r,t);else if(n.child!==null){n.child.return=n,n=n.child;continue}if(n===t)break e;for(;n.sibling===null;){if(n.return===null||n.return===t)break e;n=n.return}n.sibling.return=n.return,n=n.sibling}s&=1}if(q(Q,s),!(t.mode&1))t.memoizedState=null;else switch(l){case"forwards":for(r=t.child,l=null;r!==null;)n=r.alternate,n!==null&&fs(n)===null&&(l=r),r=r.sibling;r=l,r===null?(l=t.child,t.child=null):(l=r.sibling,r.sibling=null),ml(t,!1,l,r,i);break;case"backwards":for(r=null,l=t.child,t.child=null;l!==null;){if(n=l.alternate,n!==null&&fs(n)===null){t.child=l;break}n=l.sibling,l.sibling=r,r=l,l=n}ml(t,!0,r,null,i);break;case"together":ml(t,!1,null,null,void 0);break;default:t.memoizedState=null}return t.child}function Yr(n,t){!(t.mode&1)&&n!==null&&(n.alternate=null,t.alternate=null,t.flags|=2)}function cn(n,t,r){if(n!==null&&(t.dependencies=n.dependencies),qn|=t.lanes,!(r&t.childLanes))return null;if(n!==null&&t.child!==n.child)throw Error(T(153));if(t.child!==null){for(n=t.child,r=Tn(n,n.pendingProps),t.child=r,r.return=t;n.sibling!==null;)n=n.sibling,r=r.sibling=Tn(n,n.pendingProps),r.return=t;r.sibling=null}return t.child}function ym(n,t,r){switch(t.tag){case 3:Td(t),vt();break;case 5:nd(t);break;case 1:Ee(t.type)&&cs(t);break;case 4:$i(t,t.stateNode.containerInfo);break;case 10:var s=t.type._context,l=t.memoizedProps.value;q(hs,s._currentValue),s._currentValue=l;break;case 13:if(s=t.memoizedState,s!==null)return s.dehydrated!==null?(q(Q,Q.current&1),t.flags|=128,null):r&t.child.childLanes?Rd(n,t,r):(q(Q,Q.current&1),n=cn(n,t,r),n!==null?n.sibling:null);q(Q,Q.current&1);break;case 19:if(s=(r&t.childLanes)!==0,n.flags&128){if(s)return Cd(n,t,r);t.flags|=128}if(l=t.memoizedState,l!==null&&(l.rendering=null,l.tail=null,l.lastEffect=null),q(Q,Q.current),s)break;return null;case 22:case 23:return t.lanes=0,Ed(n,t,r)}return cn(n,t,r)}var _d,ti,Pd,Ad;_d=function(n,t){for(var r=t.child;r!==null;){if(r.tag===5||r.tag===6)n.appendChild(r.stateNode);else if(r.tag!==4&&r.child!==null){r.child.return=r,r=r.child;continue}if(r===t)break;for(;r.sibling===null;){if(r.return===null||r.return===t)return;r=r.return}r.sibling.return=r.return,r=r.sibling}};ti=function(){};Pd=function(n,t,r,s){var l=n.memoizedProps;if(l!==s){n=t.stateNode,Dn(Je.current);var i=null;switch(r){case"input":l=wl(n,l),s=wl(n,s),i=[];break;case"select":l=Z({},l,{value:void 0}),s=Z({},s,{value:void 0}),i=[];break;case"textarea":l=Tl(n,l),s=Tl(n,s),i=[];break;default:typeof l.onClick!="function"&&typeof s.onClick=="function"&&(n.onclick=os)}Cl(r,s);var o;r=null;for(u in l)if(!s.hasOwnProperty(u)&&l.hasOwnProperty(u)&&l[u]!=null)if(u==="style"){var a=l[u];for(o in a)a.hasOwnProperty(o)&&(r||(r={}),r[o]="")}else u!=="dangerouslySetInnerHTML"&&u!=="children"&&u!=="suppressContentEditableWarning"&&u!=="suppressHydrationWarning"&&u!=="autoFocus"&&(Qt.hasOwnProperty(u)?i||(i=[]):(i=i||[]).push(u,null));for(u in s){var c=s[u];if(a=l!=null?l[u]:void 0,s.hasOwnProperty(u)&&c!==a&&(c!=null||a!=null))if(u==="style")if(a){for(o in a)!a.hasOwnProperty(o)||c&&c.hasOwnProperty(o)||(r||(r={}),r[o]="");for(o in c)c.hasOwnProperty(o)&&a[o]!==c[o]&&(r||(r={}),r[o]=c[o])}else r||(i||(i=[]),i.push(u,r)),r=c;else u==="dangerouslySetInnerHTML"?(c=c?c.__html:void 0,a=a?a.__html:void 0,c!=null&&a!==c&&(i=i||[]).push(u,c)):u==="children"?typeof c!="string"&&typeof c!="number"||(i=i||[]).push(u,""+c):u!=="suppressContentEditableWarning"&&u!=="suppressHydrationWarning"&&(Qt.hasOwnProperty(u)?(c!=null&&u==="onScroll"&&W("scroll",n),i||a===c||(i=[])):(i=i||[]).push(u,c))}r&&(i=i||[]).push("style",r);var u=i;(t.updateQueue=u)&&(t.flags|=4)}};Ad=function(n,t,r,s){r!==s&&(t.flags|=4)};function Mt(n,t){if(!Y)switch(n.tailMode){case"hidden":t=n.tail;for(var r=null;t!==null;)t.alternate!==null&&(r=t),t=t.sibling;r===null?n.tail=null:r.sibling=null;break;case"collapsed":r=n.tail;for(var s=null;r!==null;)r.alternate!==null&&(s=r),r=r.sibling;s===null?t||n.tail===null?n.tail=null:n.tail.sibling=null:s.sibling=null}}function me(n){var t=n.alternate!==null&&n.alternate.child===n.child,r=0,s=0;if(t)for(var l=n.child;l!==null;)r|=l.lanes|l.childLanes,s|=l.subtreeFlags&14680064,s|=l.flags&14680064,l.return=n,l=l.sibling;else for(l=n.child;l!==null;)r|=l.lanes|l.childLanes,s|=l.subtreeFlags,s|=l.flags,l.return=n,l=l.sibling;return n.subtreeFlags|=s,n.childLanes=r,t}function km(n,t,r){var s=t.pendingProps;switch(Li(t),t.tag){case 2:case 16:case 15:case 0:case 11:case 7:case 8:case 12:case 9:case 14:return me(t),null;case 1:return Ee(t.type)&&as(),me(t),null;case 3:return s=t.stateNode,kt(),V(we),V(xe),qi(),s.pendingContext&&(s.context=s.pendingContext,s.pendingContext=null),(n===null||n.child===null)&&(Mr(t)?t.flags|=4:n===null||n.memoizedState.isDehydrated&&!(t.flags&256)||(t.flags|=1024,qe!==null&&(di(qe),qe=null))),ti(n,t),me(t),null;case 5:Hi(t);var l=Dn(ar.current);if(r=t.type,n!==null&&t.stateNode!=null)Pd(n,t,r,s,l),n.ref!==t.ref&&(t.flags|=512,t.flags|=2097152);else{if(!s){if(t.stateNode===null)throw Error(T(166));return me(t),null}if(n=Dn(Je.current),Mr(t)){s=t.stateNode,r=t.type;var i=t.memoizedProps;switch(s[Xe]=t,s[ir]=i,n=(t.mode&1)!==0,r){case"dialog":W("cancel",s),W("close",s);break;case"iframe":case"object":case"embed":W("load",s);break;case"video":case"audio":for(l=0;l<Kt.length;l++)W(Kt[l],s);break;case"source":W("error",s);break;case"img":case"image":case"link":W("error",s),W("load",s);break;case"details":W("toggle",s);break;case"input":ko(s,i),W("invalid",s);break;case"select":s._wrapperState={wasMultiple:!!i.multiple},W("invalid",s);break;case"textarea":bo(s,i),W("invalid",s)}Cl(r,i),l=null;for(var o in i)if(i.hasOwnProperty(o)){var a=i[o];o==="children"?typeof a=="string"?s.textContent!==a&&(i.suppressHydrationWarning!==!0&&Or(s.textContent,a,n),l=["children",a]):typeof a=="number"&&s.textContent!==""+a&&(i.suppressHydrationWarning!==!0&&Or(s.textContent,a,n),l=["children",""+a]):Qt.hasOwnProperty(o)&&a!=null&&o==="onScroll"&&W("scroll",s)}switch(r){case"input":Sr(s),No(s,i,!0);break;case"textarea":Sr(s),wo(s);break;case"select":case"option":break;default:typeof i.onClick=="function"&&(s.onclick=os)}s=l,t.updateQueue=s,s!==null&&(t.flags|=4)}else{o=l.nodeType===9?l:l.ownerDocument,n==="http://www.w3.org/1999/xhtml"&&(n=ic(r)),n==="http://www.w3.org/1999/xhtml"?r==="script"?(n=o.createElement("div"),n.innerHTML="<script><\/script>",n=n.removeChild(n.firstChild)):typeof s.is=="string"?n=o.createElement(r,{is:s.is}):(n=o.createElement(r),r==="select"&&(o=n,s.multiple?o.multiple=!0:s.size&&(o.size=s.size))):n=o.createElementNS(n,r),n[Xe]=t,n[ir]=s,_d(n,t,!1,!1),t.stateNode=n;e:{switch(o=_l(r,s),r){case"dialog":W("cancel",n),W("close",n),l=s;break;case"iframe":case"object":case"embed":W("load",n),l=s;break;case"video":case"audio":for(l=0;l<Kt.length;l++)W(Kt[l],n);l=s;break;case"source":W("error",n),l=s;break;case"img":case"image":case"link":W("error",n),W("load",n),l=s;break;case"details":W("toggle",n),l=s;break;case"input":ko(n,s),l=wl(n,s),W("invalid",n);break;case"option":l=s;break;case"select":n._wrapperState={wasMultiple:!!s.multiple},l=Z({},s,{value:void 0}),W("invalid",n);break;case"textarea":bo(n,s),l=Tl(n,s),W("invalid",n);break;default:l=s}Cl(r,l),a=l;for(i in a)if(a.hasOwnProperty(i)){var c=a[i];i==="style"?cc(n,c):i==="dangerouslySetInnerHTML"?(c=c?c.__html:void 0,c!=null&&oc(n,c)):i==="children"?typeof c=="string"?(r!=="textarea"||c!=="")&&Xt(n,c):typeof c=="number"&&Xt(n,""+c):i!=="suppressContentEditableWarning"&&i!=="suppressHydrationWarning"&&i!=="autoFocus"&&(Qt.hasOwnProperty(i)?c!=null&&i==="onScroll"&&W("scroll",n):c!=null&&ki(n,i,c,o))}switch(r){case"input":Sr(n),No(n,s,!1);break;case"textarea":Sr(n),wo(n);break;case"option":s.value!=null&&n.setAttribute("value",""+Cn(s.value));break;case"select":n.multiple=!!s.multiple,i=s.value,i!=null?ut(n,!!s.multiple,i,!1):s.defaultValue!=null&&ut(n,!!s.multiple,s.defaultValue,!0);break;default:typeof l.onClick=="function"&&(n.onclick=os)}switch(r){case"button":case"input":case"select":case"textarea":s=!!s.autoFocus;break e;case"img":s=!0;break e;default:s=!1}}s&&(t.flags|=4)}t.ref!==null&&(t.flags|=512,t.flags|=2097152)}return me(t),null;case 6:if(n&&t.stateNode!=null)Ad(n,t,n.memoizedProps,s);else{if(typeof s!="string"&&t.stateNode===null)throw Error(T(166));if(r=Dn(ar.current),Dn(Je.current),Mr(t)){if(s=t.stateNode,r=t.memoizedProps,s[Xe]=t,(i=s.nodeValue!==r)&&(n=Ce,n!==null))switch(n.tag){case 3:Or(s.nodeValue,r,(n.mode&1)!==0);break;case 5:n.memoizedProps.suppressHydrationWarning!==!0&&Or(s.nodeValue,r,(n.mode&1)!==0)}i&&(t.flags|=4)}else s=(r.nodeType===9?r:r.ownerDocument).createTextNode(s),s[Xe]=t,t.stateNode=s}return me(t),null;case 13:if(V(Q),s=t.memoizedState,n===null||n.memoizedState!==null&&n.memoizedState.dehydrated!==null){if(Y&&Re!==null&&t.mode&1&&!(t.flags&128))Qc(),vt(),t.flags|=98560,i=!1;else if(i=Mr(t),s!==null&&s.dehydrated!==null){if(n===null){if(!i)throw Error(T(318));if(i=t.memoizedState,i=i!==null?i.dehydrated:null,!i)throw Error(T(317));i[Xe]=t}else vt(),!(t.flags&128)&&(t.memoizedState=null),t.flags|=4;me(t),i=!1}else qe!==null&&(di(qe),qe=null),i=!0;if(!i)return t.flags&65536?t:null}return t.flags&128?(t.lanes=r,t):(s=s!==null,s!==(n!==null&&n.memoizedState!==null)&&s&&(t.child.flags|=8192,t.mode&1&&(n===null||Q.current&1?ie===0&&(ie=3):ro())),t.updateQueue!==null&&(t.flags|=4),me(t),null);case 4:return kt(),ti(n,t),n===null&&sr(t.stateNode.containerInfo),me(t),null;case 10:return Ki(t.type._context),me(t),null;case 17:return Ee(t.type)&&as(),me(t),null;case 19:if(V(Q),i=t.memoizedState,i===null)return me(t),null;if(s=(t.flags&128)!==0,o=i.rendering,o===null)if(s)Mt(i,!1);else{if(ie!==0||n!==null&&n.flags&128)for(n=t.child;n!==null;){if(o=fs(n),o!==null){for(t.flags|=128,Mt(i,!1),s=o.updateQueue,s!==null&&(t.updateQueue=s,t.flags|=4),t.subtreeFlags=0,s=r,r=t.child;r!==null;)i=r,n=s,i.flags&=14680066,o=i.alternate,o===null?(i.childLanes=0,i.lanes=n,i.child=null,i.subtreeFlags=0,i.memoizedProps=null,i.memoizedState=null,i.updateQueue=null,i.dependencies=null,i.stateNode=null):(i.childLanes=o.childLanes,i.lanes=o.lanes,i.child=o.child,i.subtreeFlags=0,i.deletions=null,i.memoizedProps=o.memoizedProps,i.memoizedState=o.memoizedState,i.updateQueue=o.updateQueue,i.type=o.type,n=o.dependencies,i.dependencies=n===null?null:{lanes:n.lanes,firstContext:n.firstContext}),r=r.sibling;return q(Q,Q.current&1|2),t.child}n=n.sibling}i.tail!==null&&ee()>bt&&(t.flags|=128,s=!0,Mt(i,!1),t.lanes=4194304)}else{if(!s)if(n=fs(o),n!==null){if(t.flags|=128,s=!0,r=n.updateQueue,r!==null&&(t.updateQueue=r,t.flags|=4),Mt(i,!0),i.tail===null&&i.tailMode==="hidden"&&!o.alternate&&!Y)return me(t),null}else 2*ee()-i.renderingStartTime>bt&&r!==1073741824&&(t.flags|=128,s=!0,Mt(i,!1),t.lanes=4194304);i.isBackwards?(o.sibling=t.child,t.child=o):(r=i.last,r!==null?r.sibling=o:t.child=o,i.last=o)}return i.tail!==null?(t=i.tail,i.rendering=t,i.tail=t.sibling,i.renderingStartTime=ee(),t.sibling=null,r=Q.current,q(Q,s?r&1|2:r&1),t):(me(t),null);case 22:case 23:return to(),s=t.memoizedState!==null,n!==null&&n.memoizedState!==null!==s&&(t.flags|=8192),s&&t.mode&1?Te&1073741824&&(me(t),t.subtreeFlags&6&&(t.flags|=8192)):me(t),null;case 24:return null;case 25:return null}throw Error(T(156,t.tag))}function Nm(n,t){switch(Li(t),t.tag){case 1:return Ee(t.type)&&as(),n=t.flags,n&65536?(t.flags=n&-65537|128,t):null;case 3:return kt(),V(we),V(xe),qi(),n=t.flags,n&65536&&!(n&128)?(t.flags=n&-65537|128,t):null;case 5:return Hi(t),null;case 13:if(V(Q),n=t.memoizedState,n!==null&&n.dehydrated!==null){if(t.alternate===null)throw Error(T(340));vt()}return n=t.flags,n&65536?(t.flags=n&-65537|128,t):null;case 19:return V(Q),null;case 4:return kt(),null;case 10:return Ki(t.type._context),null;case 22:case 23:return to(),null;case 24:return null;default:return null}}var Dr=!1,pe=!1,bm=typeof WeakSet=="function"?WeakSet:Set,P=null;function ct(n,t){var r=n.ref;if(r!==null)if(typeof r=="function")try{r(null)}catch(s){J(n,t,s)}else r.current=null}function ri(n,t,r){try{r()}catch(s){J(n,t,s)}}var ma=!1;function wm(n,t){if(Kl=ss,n=Fc(),Oi(n)){if("selectionStart"in n)var r={start:n.selectionStart,end:n.selectionEnd};else e:{r=(r=n.ownerDocument)&&r.defaultView||window;var s=r.getSelection&&r.getSelection();if(s&&s.rangeCount!==0){r=s.anchorNode;var l=s.anchorOffset,i=s.focusNode;s=s.focusOffset;try{r.nodeType,i.nodeType}catch{r=null;break e}var o=0,a=-1,c=-1,u=0,x=0,m=n,f=null;n:for(;;){for(var k;m!==r||l!==0&&m.nodeType!==3||(a=o+l),m!==i||s!==0&&m.nodeType!==3||(c=o+s),m.nodeType===3&&(o+=m.nodeValue.length),(k=m.firstChild)!==null;)f=m,m=k;for(;;){if(m===n)break n;if(f===r&&++u===l&&(a=o),f===i&&++x===s&&(c=o),(k=m.nextSibling)!==null)break;m=f,f=m.parentNode}m=k}r=a===-1||c===-1?null:{start:a,end:c}}else r=null}r=r||{start:0,end:0}}else r=null;for(Bl={focusedElem:n,selectionRange:r},ss=!1,P=t;P!==null;)if(t=P,n=t.child,(t.subtreeFlags&1028)!==0&&n!==null)n.return=t,P=n;else for(;P!==null;){t=P;try{var N=t.alternate;if(t.flags&1024)switch(t.tag){case 0:case 11:case 15:break;case 1:if(N!==null){var g=N.memoizedProps,w=N.memoizedState,p=t.stateNode,d=p.getSnapshotBeforeUpdate(t.elementType===t.type?g:$e(t.type,g),w);p.__reactInternalSnapshotBeforeUpdate=d}break;case 3:var h=t.stateNode.containerInfo;h.nodeType===1?h.textContent="":h.nodeType===9&&h.documentElement&&h.removeChild(h.documentElement);break;case 5:case 6:case 4:case 17:break;default:throw Error(T(163))}}catch(j){J(t,t.return,j)}if(n=t.sibling,n!==null){n.return=t.return,P=n;break}P=t.return}return N=ma,ma=!1,N}function Wt(n,t,r){var s=t.updateQueue;if(s=s!==null?s.lastEffect:null,s!==null){var l=s=s.next;do{if((l.tag&n)===n){var i=l.destroy;l.destroy=void 0,i!==void 0&&ri(t,r,i)}l=l.next}while(l!==s)}}function Is(n,t){if(t=t.updateQueue,t=t!==null?t.lastEffect:null,t!==null){var r=t=t.next;do{if((r.tag&n)===n){var s=r.create;r.destroy=s()}r=r.next}while(r!==t)}}function si(n){var t=n.ref;if(t!==null){var r=n.stateNode;switch(n.tag){case 5:n=r;break;default:n=r}typeof t=="function"?t(n):t.current=n}}function Id(n){var t=n.alternate;t!==null&&(n.alternate=null,Id(t)),n.child=null,n.deletions=null,n.sibling=null,n.tag===5&&(t=n.stateNode,t!==null&&(delete t[Xe],delete t[ir],delete t[Hl],delete t[im],delete t[om])),n.stateNode=null,n.return=null,n.dependencies=null,n.memoizedProps=null,n.memoizedState=null,n.pendingProps=null,n.stateNode=null,n.updateQueue=null}function Od(n){return n.tag===5||n.tag===3||n.tag===4}function pa(n){e:for(;;){for(;n.sibling===null;){if(n.return===null||Od(n.return))return null;n=n.return}for(n.sibling.return=n.return,n=n.sibling;n.tag!==5&&n.tag!==6&&n.tag!==18;){if(n.flags&2||n.child===null||n.tag===4)continue e;n.child.return=n,n=n.child}if(!(n.flags&2))return n.stateNode}}function li(n,t,r){var s=n.tag;if(s===5||s===6)n=n.stateNode,t?r.nodeType===8?r.parentNode.insertBefore(n,t):r.insertBefore(n,t):(r.nodeType===8?(t=r.parentNode,t.insertBefore(n,r)):(t=r,t.appendChild(n)),r=r._reactRootContainer,r!=null||t.onclick!==null||(t.onclick=os));else if(s!==4&&(n=n.child,n!==null))for(li(n,t,r),n=n.sibling;n!==null;)li(n,t,r),n=n.sibling}function ii(n,t,r){var s=n.tag;if(s===5||s===6)n=n.stateNode,t?r.insertBefore(n,t):r.appendChild(n);else if(s!==4&&(n=n.child,n!==null))for(ii(n,t,r),n=n.sibling;n!==null;)ii(n,t,r),n=n.sibling}var ce=null,He=!1;function un(n,t,r){for(r=r.child;r!==null;)Md(n,t,r),r=r.sibling}function Md(n,t,r){if(Ze&&typeof Ze.onCommitFiberUnmount=="function")try{Ze.onCommitFiberUnmount(Es,r)}catch{}switch(r.tag){case 5:pe||ct(r,t);case 6:var s=ce,l=He;ce=null,un(n,t,r),ce=s,He=l,ce!==null&&(He?(n=ce,r=r.stateNode,n.nodeType===8?n.parentNode.removeChild(r):n.removeChild(r)):ce.removeChild(r.stateNode));break;case 18:ce!==null&&(He?(n=ce,r=r.stateNode,n.nodeType===8?il(n.parentNode,r):n.nodeType===1&&il(n,r),nr(n)):il(ce,r.stateNode));break;case 4:s=ce,l=He,ce=r.stateNode.containerInfo,He=!0,un(n,t,r),ce=s,He=l;break;case 0:case 11:case 14:case 15:if(!pe&&(s=r.updateQueue,s!==null&&(s=s.lastEffect,s!==null))){l=s=s.next;do{var i=l,o=i.destroy;i=i.tag,o!==void 0&&(i&2||i&4)&&ri(r,t,o),l=l.next}while(l!==s)}un(n,t,r);break;case 1:if(!pe&&(ct(r,t),s=r.stateNode,typeof s.componentWillUnmount=="function"))try{s.props=r.memoizedProps,s.state=r.memoizedState,s.componentWillUnmount()}catch(a){J(r,t,a)}un(n,t,r);break;case 21:un(n,t,r);break;case 22:r.mode&1?(pe=(s=pe)||r.memoizedState!==null,un(n,t,r),pe=s):un(n,t,r);break;default:un(n,t,r)}}function fa(n){var t=n.updateQueue;if(t!==null){n.updateQueue=null;var r=n.stateNode;r===null&&(r=n.stateNode=new bm),t.forEach(function(s){var l=Im.bind(null,n,s);r.has(s)||(r.add(s),s.then(l,l))})}}function ze(n,t){var r=t.deletions;if(r!==null)for(var s=0;s<r.length;s++){var l=r[s];try{var i=n,o=t,a=o;e:for(;a!==null;){switch(a.tag){case 5:ce=a.stateNode,He=!1;break e;case 3:ce=a.stateNode.containerInfo,He=!0;break e;case 4:ce=a.stateNode.containerInfo,He=!0;break e}a=a.return}if(ce===null)throw Error(T(160));Md(i,o,l),ce=null,He=!1;var c=l.alternate;c!==null&&(c.return=null),l.return=null}catch(u){J(l,t,u)}}if(t.subtreeFlags&12854)for(t=t.child;t!==null;)Ld(t,n),t=t.sibling}function Ld(n,t){var r=n.alternate,s=n.flags;switch(n.tag){case 0:case 11:case 14:case 15:if(ze(t,n),Ye(n),s&4){try{Wt(3,n,n.return),Is(3,n)}catch(g){J(n,n.return,g)}try{Wt(5,n,n.return)}catch(g){J(n,n.return,g)}}break;case 1:ze(t,n),Ye(n),s&512&&r!==null&&ct(r,r.return);break;case 5:if(ze(t,n),Ye(n),s&512&&r!==null&&ct(r,r.return),n.flags&32){var l=n.stateNode;try{Xt(l,"")}catch(g){J(n,n.return,g)}}if(s&4&&(l=n.stateNode,l!=null)){var i=n.memoizedProps,o=r!==null?r.memoizedProps:i,a=n.type,c=n.updateQueue;if(n.updateQueue=null,c!==null)try{a==="input"&&i.type==="radio"&&i.name!=null&&sc(l,i),_l(a,o);var u=_l(a,i);for(o=0;o<c.length;o+=2){var x=c[o],m=c[o+1];x==="style"?cc(l,m):x==="dangerouslySetInnerHTML"?oc(l,m):x==="children"?Xt(l,m):ki(l,x,m,u)}switch(a){case"input":El(l,i);break;case"textarea":lc(l,i);break;case"select":var f=l._wrapperState.wasMultiple;l._wrapperState.wasMultiple=!!i.multiple;var k=i.value;k!=null?ut(l,!!i.multiple,k,!1):f!==!!i.multiple&&(i.defaultValue!=null?ut(l,!!i.multiple,i.defaultValue,!0):ut(l,!!i.multiple,i.multiple?[]:"",!1))}l[ir]=i}catch(g){J(n,n.return,g)}}break;case 6:if(ze(t,n),Ye(n),s&4){if(n.stateNode===null)throw Error(T(162));l=n.stateNode,i=n.memoizedProps;try{l.nodeValue=i}catch(g){J(n,n.return,g)}}break;case 3:if(ze(t,n),Ye(n),s&4&&r!==null&&r.memoizedState.isDehydrated)try{nr(t.containerInfo)}catch(g){J(n,n.return,g)}break;case 4:ze(t,n),Ye(n);break;case 13:ze(t,n),Ye(n),l=n.child,l.flags&8192&&(i=l.memoizedState!==null,l.stateNode.isHidden=i,!i||l.alternate!==null&&l.alternate.memoizedState!==null||(eo=ee())),s&4&&fa(n);break;case 22:if(x=r!==null&&r.memoizedState!==null,n.mode&1?(pe=(u=pe)||x,ze(t,n),pe=u):ze(t,n),Ye(n),s&8192){if(u=n.memoizedState!==null,(n.stateNode.isHidden=u)&&!x&&n.mode&1)for(P=n,x=n.child;x!==null;){for(m=P=x;P!==null;){switch(f=P,k=f.child,f.tag){case 0:case 11:case 14:case 15:Wt(4,f,f.return);break;case 1:ct(f,f.return);var N=f.stateNode;if(typeof N.componentWillUnmount=="function"){s=f,r=f.return;try{t=s,N.props=t.memoizedProps,N.state=t.memoizedState,N.componentWillUnmount()}catch(g){J(s,r,g)}}break;case 5:ct(f,f.return);break;case 22:if(f.memoizedState!==null){ja(m);continue}}k!==null?(k.return=f,P=k):ja(m)}x=x.sibling}e:for(x=null,m=n;;){if(m.tag===5){if(x===null){x=m;try{l=m.stateNode,u?(i=l.style,typeof i.setProperty=="function"?i.setProperty("display","none","important"):i.display="none"):(a=m.stateNode,c=m.memoizedProps.style,o=c!=null&&c.hasOwnProperty("display")?c.display:null,a.style.display=ac("display",o))}catch(g){J(n,n.return,g)}}}else if(m.tag===6){if(x===null)try{m.stateNode.nodeValue=u?"":m.memoizedProps}catch(g){J(n,n.return,g)}}else if((m.tag!==22&&m.tag!==23||m.memoizedState===null||m===n)&&m.child!==null){m.child.return=m,m=m.child;continue}if(m===n)break e;for(;m.sibling===null;){if(m.return===null||m.return===n)break e;x===m&&(x=null),m=m.return}x===m&&(x=null),m.sibling.return=m.return,m=m.sibling}}break;case 19:ze(t,n),Ye(n),s&4&&fa(n);break;case 21:break;default:ze(t,n),Ye(n)}}function Ye(n){var t=n.flags;if(t&2){try{e:{for(var r=n.return;r!==null;){if(Od(r)){var s=r;break e}r=r.return}throw Error(T(160))}switch(s.tag){case 5:var l=s.stateNode;s.flags&32&&(Xt(l,""),s.flags&=-33);var i=pa(n);ii(n,i,l);break;case 3:case 4:var o=s.stateNode.containerInfo,a=pa(n);li(n,a,o);break;default:throw Error(T(161))}}catch(c){J(n,n.return,c)}n.flags&=-3}t&4096&&(n.flags&=-4097)}function Em(n,t,r){P=n,Fd(n)}function Fd(n,t,r){for(var s=(n.mode&1)!==0;P!==null;){var l=P,i=l.child;if(l.tag===22&&s){var o=l.memoizedState!==null||Dr;if(!o){var a=l.alternate,c=a!==null&&a.memoizedState!==null||pe;a=Dr;var u=pe;if(Dr=o,(pe=c)&&!u)for(P=l;P!==null;)o=P,c=o.child,o.tag===22&&o.memoizedState!==null?ga(l):c!==null?(c.return=o,P=c):ga(l);for(;i!==null;)P=i,Fd(i),i=i.sibling;P=l,Dr=a,pe=u}xa(n)}else l.subtreeFlags&8772&&i!==null?(i.return=l,P=i):xa(n)}}function xa(n){for(;P!==null;){var t=P;if(t.flags&8772){var r=t.alternate;try{if(t.flags&8772)switch(t.tag){case 0:case 11:case 15:pe||Is(5,t);break;case 1:var s=t.stateNode;if(t.flags&4&&!pe)if(r===null)s.componentDidMount();else{var l=t.elementType===t.type?r.memoizedProps:$e(t.type,r.memoizedProps);s.componentDidUpdate(l,r.memoizedState,s.__reactInternalSnapshotBeforeUpdate)}var i=t.updateQueue;i!==null&&ea(t,i,s);break;case 3:var o=t.updateQueue;if(o!==null){if(r=null,t.child!==null)switch(t.child.tag){case 5:r=t.child.stateNode;break;case 1:r=t.child.stateNode}ea(t,o,r)}break;case 5:var a=t.stateNode;if(r===null&&t.flags&4){r=a;var c=t.memoizedProps;switch(t.type){case"button":case"input":case"select":case"textarea":c.autoFocus&&r.focus();break;case"img":c.src&&(r.src=c.src)}}break;case 6:break;case 4:break;case 12:break;case 13:if(t.memoizedState===null){var u=t.alternate;if(u!==null){var x=u.memoizedState;if(x!==null){var m=x.dehydrated;m!==null&&nr(m)}}}break;case 19:case 17:case 21:case 22:case 23:case 25:break;default:throw Error(T(163))}pe||t.flags&512&&si(t)}catch(f){J(t,t.return,f)}}if(t===n){P=null;break}if(r=t.sibling,r!==null){r.return=t.return,P=r;break}P=t.return}}function ja(n){for(;P!==null;){var t=P;if(t===n){P=null;break}var r=t.sibling;if(r!==null){r.return=t.return,P=r;break}P=t.return}}function ga(n){for(;P!==null;){var t=P;try{switch(t.tag){case 0:case 11:case 15:var r=t.return;try{Is(4,t)}catch(c){J(t,r,c)}break;case 1:var s=t.stateNode;if(typeof s.componentDidMount=="function"){var l=t.return;try{s.componentDidMount()}catch(c){J(t,l,c)}}var i=t.return;try{si(t)}catch(c){J(t,i,c)}break;case 5:var o=t.return;try{si(t)}catch(c){J(t,o,c)}}}catch(c){J(t,t.return,c)}if(t===n){P=null;break}var a=t.sibling;if(a!==null){a.return=t.return,P=a;break}P=t.return}}var Sm=Math.ceil,gs=dn.ReactCurrentDispatcher,Zi=dn.ReactCurrentOwner,Ue=dn.ReactCurrentBatchConfig,K=0,ae=null,te=null,de=0,Te=0,dt=An(0),ie=0,hr=null,qn=0,Os=0,Ji=0,Vt=null,Ne=null,eo=0,bt=1/0,en=null,vs=!1,oi=null,En=null,Ur=!1,gn=null,ys=0,Yt=0,ai=null,Qr=-1,Xr=0;function ve(){return K&6?ee():Qr!==-1?Qr:Qr=ee()}function Sn(n){return n.mode&1?K&2&&de!==0?de&-de:cm.transition!==null?(Xr===0&&(Xr=kc()),Xr):(n=H,n!==0||(n=window.event,n=n===void 0?16:Rc(n.type)),n):1}function We(n,t,r,s){if(50<Yt)throw Yt=0,ai=null,Error(T(185));xr(n,r,s),(!(K&2)||n!==ae)&&(n===ae&&(!(K&2)&&(Os|=r),ie===4&&xn(n,de)),Se(n,s),r===1&&K===0&&!(t.mode&1)&&(bt=ee()+500,_s&&In()))}function Se(n,t){var r=n.callbackNode;ch(n,t);var s=rs(n,n===ae?de:0);if(s===0)r!==null&&To(r),n.callbackNode=null,n.callbackPriority=0;else if(t=s&-s,n.callbackPriority!==t){if(r!=null&&To(r),t===1)n.tag===0?am(va.bind(null,n)):Wc(va.bind(null,n)),sm(function(){!(K&6)&&In()}),r=null;else{switch(Nc(s)){case 1:r=Si;break;case 4:r=vc;break;case 16:r=ts;break;case 536870912:r=yc;break;default:r=ts}r=qd(r,Dd.bind(null,n))}n.callbackPriority=t,n.callbackNode=r}}function Dd(n,t){if(Qr=-1,Xr=0,K&6)throw Error(T(327));var r=n.callbackNode;if(xt()&&n.callbackNode!==r)return null;var s=rs(n,n===ae?de:0);if(s===0)return null;if(s&30||s&n.expiredLanes||t)t=ks(n,s);else{t=s;var l=K;K|=2;var i=Kd();(ae!==n||de!==t)&&(en=null,bt=ee()+500,Un(n,t));do try{Cm();break}catch(a){Ud(n,a)}while(!0);Ui(),gs.current=i,K=l,te!==null?t=0:(ae=null,de=0,t=ie)}if(t!==0){if(t===2&&(l=Ml(n),l!==0&&(s=l,t=ci(n,l))),t===1)throw r=hr,Un(n,0),xn(n,s),Se(n,ee()),r;if(t===6)xn(n,s);else{if(l=n.current.alternate,!(s&30)&&!Tm(l)&&(t=ks(n,s),t===2&&(i=Ml(n),i!==0&&(s=i,t=ci(n,i))),t===1))throw r=hr,Un(n,0),xn(n,s),Se(n,ee()),r;switch(n.finishedWork=l,n.finishedLanes=s,t){case 0:case 1:throw Error(T(345));case 2:Mn(n,Ne,en);break;case 3:if(xn(n,s),(s&130023424)===s&&(t=eo+500-ee(),10<t)){if(rs(n,0)!==0)break;if(l=n.suspendedLanes,(l&s)!==s){ve(),n.pingedLanes|=n.suspendedLanes&l;break}n.timeoutHandle=$l(Mn.bind(null,n,Ne,en),t);break}Mn(n,Ne,en);break;case 4:if(xn(n,s),(s&4194240)===s)break;for(t=n.eventTimes,l=-1;0<s;){var o=31-Ge(s);i=1<<o,o=t[o],o>l&&(l=o),s&=~i}if(s=l,s=ee()-s,s=(120>s?120:480>s?480:1080>s?1080:1920>s?1920:3e3>s?3e3:4320>s?4320:1960*Sm(s/1960))-s,10<s){n.timeoutHandle=$l(Mn.bind(null,n,Ne,en),s);break}Mn(n,Ne,en);break;case 5:Mn(n,Ne,en);break;default:throw Error(T(329))}}}return Se(n,ee()),n.callbackNode===r?Dd.bind(null,n):null}function ci(n,t){var r=Vt;return n.current.memoizedState.isDehydrated&&(Un(n,t).flags|=256),n=ks(n,t),n!==2&&(t=Ne,Ne=r,t!==null&&di(t)),n}function di(n){Ne===null?Ne=n:Ne.push.apply(Ne,n)}function Tm(n){for(var t=n;;){if(t.flags&16384){var r=t.updateQueue;if(r!==null&&(r=r.stores,r!==null))for(var s=0;s<r.length;s++){var l=r[s],i=l.getSnapshot;l=l.value;try{if(!Ve(i(),l))return!1}catch{return!1}}}if(r=t.child,t.subtreeFlags&16384&&r!==null)r.return=t,t=r;else{if(t===n)break;for(;t.sibling===null;){if(t.return===null||t.return===n)return!0;t=t.return}t.sibling.return=t.return,t=t.sibling}}return!0}function xn(n,t){for(t&=~Ji,t&=~Os,n.suspendedLanes|=t,n.pingedLanes&=~t,n=n.expirationTimes;0<t;){var r=31-Ge(t),s=1<<r;n[r]=-1,t&=~s}}function va(n){if(K&6)throw Error(T(327));xt();var t=rs(n,0);if(!(t&1))return Se(n,ee()),null;var r=ks(n,t);if(n.tag!==0&&r===2){var s=Ml(n);s!==0&&(t=s,r=ci(n,s))}if(r===1)throw r=hr,Un(n,0),xn(n,t),Se(n,ee()),r;if(r===6)throw Error(T(345));return n.finishedWork=n.current.alternate,n.finishedLanes=t,Mn(n,Ne,en),Se(n,ee()),null}function no(n,t){var r=K;K|=1;try{return n(t)}finally{K=r,K===0&&(bt=ee()+500,_s&&In())}}function Gn(n){gn!==null&&gn.tag===0&&!(K&6)&&xt();var t=K;K|=1;var r=Ue.transition,s=H;try{if(Ue.transition=null,H=1,n)return n()}finally{H=s,Ue.transition=r,K=t,!(K&6)&&In()}}function to(){Te=dt.current,V(dt)}function Un(n,t){n.finishedWork=null,n.finishedLanes=0;var r=n.timeoutHandle;if(r!==-1&&(n.timeoutHandle=-1,rm(r)),te!==null)for(r=te.return;r!==null;){var s=r;switch(Li(s),s.tag){case 1:s=s.type.childContextTypes,s!=null&&as();break;case 3:kt(),V(we),V(xe),qi();break;case 5:Hi(s);break;case 4:kt();break;case 13:V(Q);break;case 19:V(Q);break;case 10:Ki(s.type._context);break;case 22:case 23:to()}r=r.return}if(ae=n,te=n=Tn(n.current,null),de=Te=t,ie=0,hr=null,Ji=Os=qn=0,Ne=Vt=null,Fn!==null){for(t=0;t<Fn.length;t++)if(r=Fn[t],s=r.interleaved,s!==null){r.interleaved=null;var l=s.next,i=r.pending;if(i!==null){var o=i.next;i.next=l,s.next=o}r.pending=s}Fn=null}return n}function Ud(n,t){do{var r=te;try{if(Ui(),Wr.current=js,xs){for(var s=X.memoizedState;s!==null;){var l=s.queue;l!==null&&(l.pending=null),s=s.next}xs=!1}if(Hn=0,oe=le=X=null,Gt=!1,cr=0,Zi.current=null,r===null||r.return===null){ie=1,hr=t,te=null;break}e:{var i=n,o=r.return,a=r,c=t;if(t=de,a.flags|=32768,c!==null&&typeof c=="object"&&typeof c.then=="function"){var u=c,x=a,m=x.tag;if(!(x.mode&1)&&(m===0||m===11||m===15)){var f=x.alternate;f?(x.updateQueue=f.updateQueue,x.memoizedState=f.memoizedState,x.lanes=f.lanes):(x.updateQueue=null,x.memoizedState=null)}var k=ia(o);if(k!==null){k.flags&=-257,oa(k,o,a,i,t),k.mode&1&&la(i,u,t),t=k,c=u;var N=t.updateQueue;if(N===null){var g=new Set;g.add(c),t.updateQueue=g}else N.add(c);break e}else{if(!(t&1)){la(i,u,t),ro();break e}c=Error(T(426))}}else if(Y&&a.mode&1){var w=ia(o);if(w!==null){!(w.flags&65536)&&(w.flags|=256),oa(w,o,a,i,t),Fi(Nt(c,a));break e}}i=c=Nt(c,a),ie!==4&&(ie=2),Vt===null?Vt=[i]:Vt.push(i),i=o;do{switch(i.tag){case 3:i.flags|=65536,t&=-t,i.lanes|=t;var p=Nd(i,c,t);Jo(i,p);break e;case 1:a=c;var d=i.type,h=i.stateNode;if(!(i.flags&128)&&(typeof d.getDerivedStateFromError=="function"||h!==null&&typeof h.componentDidCatch=="function"&&(En===null||!En.has(h)))){i.flags|=65536,t&=-t,i.lanes|=t;var j=bd(i,a,t);Jo(i,j);break e}}i=i.return}while(i!==null)}zd(r)}catch(v){t=v,te===r&&r!==null&&(te=r=r.return);continue}break}while(!0)}function Kd(){var n=gs.current;return gs.current=js,n===null?js:n}function ro(){(ie===0||ie===3||ie===2)&&(ie=4),ae===null||!(qn&268435455)&&!(Os&268435455)||xn(ae,de)}function ks(n,t){var r=K;K|=2;var s=Kd();(ae!==n||de!==t)&&(en=null,Un(n,t));do try{Rm();break}catch(l){Ud(n,l)}while(!0);if(Ui(),K=r,gs.current=s,te!==null)throw Error(T(261));return ae=null,de=0,ie}function Rm(){for(;te!==null;)Bd(te)}function Cm(){for(;te!==null&&!eh();)Bd(te)}function Bd(n){var t=Hd(n.alternate,n,Te);n.memoizedProps=n.pendingProps,t===null?zd(n):te=t,Zi.current=null}function zd(n){var t=n;do{var r=t.alternate;if(n=t.return,t.flags&32768){if(r=Nm(r,t),r!==null){r.flags&=32767,te=r;return}if(n!==null)n.flags|=32768,n.subtreeFlags=0,n.deletions=null;else{ie=6,te=null;return}}else if(r=km(r,t,Te),r!==null){te=r;return}if(t=t.sibling,t!==null){te=t;return}te=t=n}while(t!==null);ie===0&&(ie=5)}function Mn(n,t,r){var s=H,l=Ue.transition;try{Ue.transition=null,H=1,_m(n,t,r,s)}finally{Ue.transition=l,H=s}return null}function _m(n,t,r,s){do xt();while(gn!==null);if(K&6)throw Error(T(327));r=n.finishedWork;var l=n.finishedLanes;if(r===null)return null;if(n.finishedWork=null,n.finishedLanes=0,r===n.current)throw Error(T(177));n.callbackNode=null,n.callbackPriority=0;var i=r.lanes|r.childLanes;if(dh(n,i),n===ae&&(te=ae=null,de=0),!(r.subtreeFlags&2064)&&!(r.flags&2064)||Ur||(Ur=!0,qd(ts,function(){return xt(),null})),i=(r.flags&15990)!==0,r.subtreeFlags&15990||i){i=Ue.transition,Ue.transition=null;var o=H;H=1;var a=K;K|=4,Zi.current=null,wm(n,r),Ld(r,n),Qh(Bl),ss=!!Kl,Bl=Kl=null,n.current=r,Em(r),nh(),K=a,H=o,Ue.transition=i}else n.current=r;if(Ur&&(Ur=!1,gn=n,ys=l),i=n.pendingLanes,i===0&&(En=null),sh(r.stateNode),Se(n,ee()),t!==null)for(s=n.onRecoverableError,r=0;r<t.length;r++)l=t[r],s(l.value,{componentStack:l.stack,digest:l.digest});if(vs)throw vs=!1,n=oi,oi=null,n;return ys&1&&n.tag!==0&&xt(),i=n.pendingLanes,i&1?n===ai?Yt++:(Yt=0,ai=n):Yt=0,In(),null}function xt(){if(gn!==null){var n=Nc(ys),t=Ue.transition,r=H;try{if(Ue.transition=null,H=16>n?16:n,gn===null)var s=!1;else{if(n=gn,gn=null,ys=0,K&6)throw Error(T(331));var l=K;for(K|=4,P=n.current;P!==null;){var i=P,o=i.child;if(P.flags&16){var a=i.deletions;if(a!==null){for(var c=0;c<a.length;c++){var u=a[c];for(P=u;P!==null;){var x=P;switch(x.tag){case 0:case 11:case 15:Wt(8,x,i)}var m=x.child;if(m!==null)m.return=x,P=m;else for(;P!==null;){x=P;var f=x.sibling,k=x.return;if(Id(x),x===u){P=null;break}if(f!==null){f.return=k,P=f;break}P=k}}}var N=i.alternate;if(N!==null){var g=N.child;if(g!==null){N.child=null;do{var w=g.sibling;g.sibling=null,g=w}while(g!==null)}}P=i}}if(i.subtreeFlags&2064&&o!==null)o.return=i,P=o;else e:for(;P!==null;){if(i=P,i.flags&2048)switch(i.tag){case 0:case 11:case 15:Wt(9,i,i.return)}var p=i.sibling;if(p!==null){p.return=i.return,P=p;break e}P=i.return}}var d=n.current;for(P=d;P!==null;){o=P;var h=o.child;if(o.subtreeFlags&2064&&h!==null)h.return=o,P=h;else e:for(o=d;P!==null;){if(a=P,a.flags&2048)try{switch(a.tag){case 0:case 11:case 15:Is(9,a)}}catch(v){J(a,a.return,v)}if(a===o){P=null;break e}var j=a.sibling;if(j!==null){j.return=a.return,P=j;break e}P=a.return}}if(K=l,In(),Ze&&typeof Ze.onPostCommitFiberRoot=="function")try{Ze.onPostCommitFiberRoot(Es,n)}catch{}s=!0}return s}finally{H=r,Ue.transition=t}}return!1}function ya(n,t,r){t=Nt(r,t),t=Nd(n,t,1),n=wn(n,t,1),t=ve(),n!==null&&(xr(n,1,t),Se(n,t))}function J(n,t,r){if(n.tag===3)ya(n,n,r);else for(;t!==null;){if(t.tag===3){ya(t,n,r);break}else if(t.tag===1){var s=t.stateNode;if(typeof t.type.getDerivedStateFromError=="function"||typeof s.componentDidCatch=="function"&&(En===null||!En.has(s))){n=Nt(r,n),n=bd(t,n,1),t=wn(t,n,1),n=ve(),t!==null&&(xr(t,1,n),Se(t,n));break}}t=t.return}}function Pm(n,t,r){var s=n.pingCache;s!==null&&s.delete(t),t=ve(),n.pingedLanes|=n.suspendedLanes&r,ae===n&&(de&r)===r&&(ie===4||ie===3&&(de&130023424)===de&&500>ee()-eo?Un(n,0):Ji|=r),Se(n,t)}function $d(n,t){t===0&&(n.mode&1?(t=Cr,Cr<<=1,!(Cr&130023424)&&(Cr=4194304)):t=1);var r=ve();n=an(n,t),n!==null&&(xr(n,t,r),Se(n,r))}function Am(n){var t=n.memoizedState,r=0;t!==null&&(r=t.retryLane),$d(n,r)}function Im(n,t){var r=0;switch(n.tag){case 13:var s=n.stateNode,l=n.memoizedState;l!==null&&(r=l.retryLane);break;case 19:s=n.stateNode;break;default:throw Error(T(314))}s!==null&&s.delete(t),$d(n,r)}var Hd;Hd=function(n,t,r){if(n!==null)if(n.memoizedProps!==t.pendingProps||we.current)be=!0;else{if(!(n.lanes&r)&&!(t.flags&128))return be=!1,ym(n,t,r);be=!!(n.flags&131072)}else be=!1,Y&&t.flags&1048576&&Vc(t,us,t.index);switch(t.lanes=0,t.tag){case 2:var s=t.type;Yr(n,t),n=t.pendingProps;var l=gt(t,xe.current);ft(t,r),l=Wi(null,t,s,n,l,r);var i=Vi();return t.flags|=1,typeof l=="object"&&l!==null&&typeof l.render=="function"&&l.$$typeof===void 0?(t.tag=1,t.memoizedState=null,t.updateQueue=null,Ee(s)?(i=!0,cs(t)):i=!1,t.memoizedState=l.state!==null&&l.state!==void 0?l.state:null,zi(t),l.updater=As,t.stateNode=l,l._reactInternals=t,Ql(t,s,n,r),t=Jl(null,t,s,!0,i,r)):(t.tag=0,Y&&i&&Mi(t),ge(null,t,l,r),t=t.child),t;case 16:s=t.elementType;e:{switch(Yr(n,t),n=t.pendingProps,l=s._init,s=l(s._payload),t.type=s,l=t.tag=Mm(s),n=$e(s,n),l){case 0:t=Zl(null,t,s,n,r);break e;case 1:t=da(null,t,s,n,r);break e;case 11:t=aa(null,t,s,n,r);break e;case 14:t=ca(null,t,s,$e(s.type,n),r);break e}throw Error(T(306,s,""))}return t;case 0:return s=t.type,l=t.pendingProps,l=t.elementType===s?l:$e(s,l),Zl(n,t,s,l,r);case 1:return s=t.type,l=t.pendingProps,l=t.elementType===s?l:$e(s,l),da(n,t,s,l,r);case 3:e:{if(Td(t),n===null)throw Error(T(387));s=t.pendingProps,i=t.memoizedState,l=i.element,ed(n,t),ps(t,s,null,r);var o=t.memoizedState;if(s=o.element,i.isDehydrated)if(i={element:s,isDehydrated:!1,cache:o.cache,pendingSuspenseBoundaries:o.pendingSuspenseBoundaries,transitions:o.transitions},t.updateQueue.baseState=i,t.memoizedState=i,t.flags&256){l=Nt(Error(T(423)),t),t=ua(n,t,s,r,l);break e}else if(s!==l){l=Nt(Error(T(424)),t),t=ua(n,t,s,r,l);break e}else for(Re=bn(t.stateNode.containerInfo.firstChild),Ce=t,Y=!0,qe=null,r=Zc(t,null,s,r),t.child=r;r;)r.flags=r.flags&-3|4096,r=r.sibling;else{if(vt(),s===l){t=cn(n,t,r);break e}ge(n,t,s,r)}t=t.child}return t;case 5:return nd(t),n===null&&Wl(t),s=t.type,l=t.pendingProps,i=n!==null?n.memoizedProps:null,o=l.children,zl(s,l)?o=null:i!==null&&zl(s,i)&&(t.flags|=32),Sd(n,t),ge(n,t,o,r),t.child;case 6:return n===null&&Wl(t),null;case 13:return Rd(n,t,r);case 4:return $i(t,t.stateNode.containerInfo),s=t.pendingProps,n===null?t.child=yt(t,null,s,r):ge(n,t,s,r),t.child;case 11:return s=t.type,l=t.pendingProps,l=t.elementType===s?l:$e(s,l),aa(n,t,s,l,r);case 7:return ge(n,t,t.pendingProps,r),t.child;case 8:return ge(n,t,t.pendingProps.children,r),t.child;case 12:return ge(n,t,t.pendingProps.children,r),t.child;case 10:e:{if(s=t.type._context,l=t.pendingProps,i=t.memoizedProps,o=l.value,q(hs,s._currentValue),s._currentValue=o,i!==null)if(Ve(i.value,o)){if(i.children===l.children&&!we.current){t=cn(n,t,r);break e}}else for(i=t.child,i!==null&&(i.return=t);i!==null;){var a=i.dependencies;if(a!==null){o=i.child;for(var c=a.firstContext;c!==null;){if(c.context===s){if(i.tag===1){c=sn(-1,r&-r),c.tag=2;var u=i.updateQueue;if(u!==null){u=u.shared;var x=u.pending;x===null?c.next=c:(c.next=x.next,x.next=c),u.pending=c}}i.lanes|=r,c=i.alternate,c!==null&&(c.lanes|=r),Vl(i.return,r,t),a.lanes|=r;break}c=c.next}}else if(i.tag===10)o=i.type===t.type?null:i.child;else if(i.tag===18){if(o=i.return,o===null)throw Error(T(341));o.lanes|=r,a=o.alternate,a!==null&&(a.lanes|=r),Vl(o,r,t),o=i.sibling}else o=i.child;if(o!==null)o.return=i;else for(o=i;o!==null;){if(o===t){o=null;break}if(i=o.sibling,i!==null){i.return=o.return,o=i;break}o=o.return}i=o}ge(n,t,l.children,r),t=t.child}return t;case 9:return l=t.type,s=t.pendingProps.children,ft(t,r),l=Ke(l),s=s(l),t.flags|=1,ge(n,t,s,r),t.child;case 14:return s=t.type,l=$e(s,t.pendingProps),l=$e(s.type,l),ca(n,t,s,l,r);case 15:return wd(n,t,t.type,t.pendingProps,r);case 17:return s=t.type,l=t.pendingProps,l=t.elementType===s?l:$e(s,l),Yr(n,t),t.tag=1,Ee(s)?(n=!0,cs(t)):n=!1,ft(t,r),kd(t,s,l),Ql(t,s,l,r),Jl(null,t,s,!0,n,r);case 19:return Cd(n,t,r);case 22:return Ed(n,t,r)}throw Error(T(156,t.tag))};function qd(n,t){return gc(n,t)}function Om(n,t,r,s){this.tag=n,this.key=r,this.sibling=this.child=this.return=this.stateNode=this.type=this.elementType=null,this.index=0,this.ref=null,this.pendingProps=t,this.dependencies=this.memoizedState=this.updateQueue=this.memoizedProps=null,this.mode=s,this.subtreeFlags=this.flags=0,this.deletions=null,this.childLanes=this.lanes=0,this.alternate=null}function De(n,t,r,s){return new Om(n,t,r,s)}function so(n){return n=n.prototype,!(!n||!n.isReactComponent)}function Mm(n){if(typeof n=="function")return so(n)?1:0;if(n!=null){if(n=n.$$typeof,n===bi)return 11;if(n===wi)return 14}return 2}function Tn(n,t){var r=n.alternate;return r===null?(r=De(n.tag,t,n.key,n.mode),r.elementType=n.elementType,r.type=n.type,r.stateNode=n.stateNode,r.alternate=n,n.alternate=r):(r.pendingProps=t,r.type=n.type,r.flags=0,r.subtreeFlags=0,r.deletions=null),r.flags=n.flags&14680064,r.childLanes=n.childLanes,r.lanes=n.lanes,r.child=n.child,r.memoizedProps=n.memoizedProps,r.memoizedState=n.memoizedState,r.updateQueue=n.updateQueue,t=n.dependencies,r.dependencies=t===null?null:{lanes:t.lanes,firstContext:t.firstContext},r.sibling=n.sibling,r.index=n.index,r.ref=n.ref,r}function Zr(n,t,r,s,l,i){var o=2;if(s=n,typeof n=="function")so(n)&&(o=1);else if(typeof n=="string")o=5;else e:switch(n){case et:return Kn(r.children,l,i,t);case Ni:o=8,l|=8;break;case yl:return n=De(12,r,t,l|2),n.elementType=yl,n.lanes=i,n;case kl:return n=De(13,r,t,l),n.elementType=kl,n.lanes=i,n;case Nl:return n=De(19,r,t,l),n.elementType=Nl,n.lanes=i,n;case nc:return Ms(r,l,i,t);default:if(typeof n=="object"&&n!==null)switch(n.$$typeof){case Ja:o=10;break e;case ec:o=9;break e;case bi:o=11;break e;case wi:o=14;break e;case mn:o=16,s=null;break e}throw Error(T(130,n==null?n:typeof n,""))}return t=De(o,r,t,l),t.elementType=n,t.type=s,t.lanes=i,t}function Kn(n,t,r,s){return n=De(7,n,s,t),n.lanes=r,n}function Ms(n,t,r,s){return n=De(22,n,s,t),n.elementType=nc,n.lanes=r,n.stateNode={isHidden:!1},n}function pl(n,t,r){return n=De(6,n,null,t),n.lanes=r,n}function fl(n,t,r){return t=De(4,n.children!==null?n.children:[],n.key,t),t.lanes=r,t.stateNode={containerInfo:n.containerInfo,pendingChildren:null,implementation:n.implementation},t}function Lm(n,t,r,s,l){this.tag=t,this.containerInfo=n,this.finishedWork=this.pingCache=this.current=this.pendingChildren=null,this.timeoutHandle=-1,this.callbackNode=this.pendingContext=this.context=null,this.callbackPriority=0,this.eventTimes=Ys(0),this.expirationTimes=Ys(-1),this.entangledLanes=this.finishedLanes=this.mutableReadLanes=this.expiredLanes=this.pingedLanes=this.suspendedLanes=this.pendingLanes=0,this.entanglements=Ys(0),this.identifierPrefix=s,this.onRecoverableError=l,this.mutableSourceEagerHydrationData=null}function lo(n,t,r,s,l,i,o,a,c){return n=new Lm(n,t,r,a,c),t===1?(t=1,i===!0&&(t|=8)):t=0,i=De(3,null,null,t),n.current=i,i.stateNode=n,i.memoizedState={element:s,isDehydrated:r,cache:null,transitions:null,pendingSuspenseBoundaries:null},zi(i),n}function Fm(n,t,r){var s=3<arguments.length&&arguments[3]!==void 0?arguments[3]:null;return{$$typeof:Jn,key:s==null?null:""+s,children:n,containerInfo:t,implementation:r}}function Gd(n){if(!n)return _n;n=n._reactInternals;e:{if(Vn(n)!==n||n.tag!==1)throw Error(T(170));var t=n;do{switch(t.tag){case 3:t=t.stateNode.context;break e;case 1:if(Ee(t.type)){t=t.stateNode.__reactInternalMemoizedMergedChildContext;break e}}t=t.return}while(t!==null);throw Error(T(171))}if(n.tag===1){var r=n.type;if(Ee(r))return Gc(n,r,t)}return t}function Wd(n,t,r,s,l,i,o,a,c){return n=lo(r,s,!0,n,l,i,o,a,c),n.context=Gd(null),r=n.current,s=ve(),l=Sn(r),i=sn(s,l),i.callback=t??null,wn(r,i,l),n.current.lanes=l,xr(n,l,s),Se(n,s),n}function Ls(n,t,r,s){var l=t.current,i=ve(),o=Sn(l);return r=Gd(r),t.context===null?t.context=r:t.pendingContext=r,t=sn(i,o),t.payload={element:n},s=s===void 0?null:s,s!==null&&(t.callback=s),n=wn(l,t,o),n!==null&&(We(n,l,o,i),Gr(n,l,o)),o}function Ns(n){if(n=n.current,!n.child)return null;switch(n.child.tag){case 5:return n.child.stateNode;default:return n.child.stateNode}}function ka(n,t){if(n=n.memoizedState,n!==null&&n.dehydrated!==null){var r=n.retryLane;n.retryLane=r!==0&&r<t?r:t}}function io(n,t){ka(n,t),(n=n.alternate)&&ka(n,t)}function Dm(){return null}var Vd=typeof reportError=="function"?reportError:function(n){console.error(n)};function oo(n){this._internalRoot=n}Fs.prototype.render=oo.prototype.render=function(n){var t=this._internalRoot;if(t===null)throw Error(T(409));Ls(n,t,null,null)};Fs.prototype.unmount=oo.prototype.unmount=function(){var n=this._internalRoot;if(n!==null){this._internalRoot=null;var t=n.containerInfo;Gn(function(){Ls(null,n,null,null)}),t[on]=null}};function Fs(n){this._internalRoot=n}Fs.prototype.unstable_scheduleHydration=function(n){if(n){var t=Ec();n={blockedOn:null,target:n,priority:t};for(var r=0;r<fn.length&&t!==0&&t<fn[r].priority;r++);fn.splice(r,0,n),r===0&&Tc(n)}};function ao(n){return!(!n||n.nodeType!==1&&n.nodeType!==9&&n.nodeType!==11)}function Ds(n){return!(!n||n.nodeType!==1&&n.nodeType!==9&&n.nodeType!==11&&(n.nodeType!==8||n.nodeValue!==" react-mount-point-unstable "))}function Na(){}function Um(n,t,r,s,l){if(l){if(typeof s=="function"){var i=s;s=function(){var u=Ns(o);i.call(u)}}var o=Wd(t,s,n,0,null,!1,!1,"",Na);return n._reactRootContainer=o,n[on]=o.current,sr(n.nodeType===8?n.parentNode:n),Gn(),o}for(;l=n.lastChild;)n.removeChild(l);if(typeof s=="function"){var a=s;s=function(){var u=Ns(c);a.call(u)}}var c=lo(n,0,!1,null,null,!1,!1,"",Na);return n._reactRootContainer=c,n[on]=c.current,sr(n.nodeType===8?n.parentNode:n),Gn(function(){Ls(t,c,r,s)}),c}function Us(n,t,r,s,l){var i=r._reactRootContainer;if(i){var o=i;if(typeof l=="function"){var a=l;l=function(){var c=Ns(o);a.call(c)}}Ls(t,o,n,l)}else o=Um(r,t,n,l,s);return Ns(o)}bc=function(n){switch(n.tag){case 3:var t=n.stateNode;if(t.current.memoizedState.isDehydrated){var r=Ut(t.pendingLanes);r!==0&&(Ti(t,r|1),Se(t,ee()),!(K&6)&&(bt=ee()+500,In()))}break;case 13:Gn(function(){var s=an(n,1);if(s!==null){var l=ve();We(s,n,1,l)}}),io(n,1)}};Ri=function(n){if(n.tag===13){var t=an(n,134217728);if(t!==null){var r=ve();We(t,n,134217728,r)}io(n,134217728)}};wc=function(n){if(n.tag===13){var t=Sn(n),r=an(n,t);if(r!==null){var s=ve();We(r,n,t,s)}io(n,t)}};Ec=function(){return H};Sc=function(n,t){var r=H;try{return H=n,t()}finally{H=r}};Al=function(n,t,r){switch(t){case"input":if(El(n,r),t=r.name,r.type==="radio"&&t!=null){for(r=n;r.parentNode;)r=r.parentNode;for(r=r.querySelectorAll("input[name="+JSON.stringify(""+t)+'][type="radio"]'),t=0;t<r.length;t++){var s=r[t];if(s!==n&&s.form===n.form){var l=Cs(s);if(!l)throw Error(T(90));rc(s),El(s,l)}}}break;case"textarea":lc(n,r);break;case"select":t=r.value,t!=null&&ut(n,!!r.multiple,t,!1)}};hc=no;mc=Gn;var Km={usingClientEntryPoint:!1,Events:[gr,st,Cs,dc,uc,no]},Lt={findFiberByHostInstance:Ln,bundleType:0,version:"18.3.1",rendererPackageName:"react-dom"},Bm={bundleType:Lt.bundleType,version:Lt.version,rendererPackageName:Lt.rendererPackageName,rendererConfig:Lt.rendererConfig,overrideHookState:null,overrideHookStateDeletePath:null,overrideHookStateRenamePath:null,overrideProps:null,overridePropsDeletePath:null,overridePropsRenamePath:null,setErrorHandler:null,setSuspenseHandler:null,scheduleUpdate:null,currentDispatcherRef:dn.ReactCurrentDispatcher,findHostInstanceByFiber:function(n){return n=xc(n),n===null?null:n.stateNode},findFiberByHostInstance:Lt.findFiberByHostInstance||Dm,findHostInstancesForRefresh:null,scheduleRefresh:null,scheduleRoot:null,setRefreshHandler:null,getCurrentFiber:null,reconcilerVersion:"18.3.1-next-f1338f8080-20240426"};if(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__<"u"){var Kr=__REACT_DEVTOOLS_GLOBAL_HOOK__;if(!Kr.isDisabled&&Kr.supportsFiber)try{Es=Kr.inject(Bm),Ze=Kr}catch{}}Pe.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED=Km;Pe.createPortal=function(n,t){var r=2<arguments.length&&arguments[2]!==void 0?arguments[2]:null;if(!ao(t))throw Error(T(200));return Fm(n,t,null,r)};Pe.createRoot=function(n,t){if(!ao(n))throw Error(T(299));var r=!1,s="",l=Vd;return t!=null&&(t.unstable_strictMode===!0&&(r=!0),t.identifierPrefix!==void 0&&(s=t.identifierPrefix),t.onRecoverableError!==void 0&&(l=t.onRecoverableError)),t=lo(n,1,!1,null,null,r,!1,s,l),n[on]=t.current,sr(n.nodeType===8?n.parentNode:n),new oo(t)};Pe.findDOMNode=function(n){if(n==null)return null;if(n.nodeType===1)return n;var t=n._reactInternals;if(t===void 0)throw typeof n.render=="function"?Error(T(188)):(n=Object.keys(n).join(","),Error(T(268,n)));return n=xc(t),n=n===null?null:n.stateNode,n};Pe.flushSync=function(n){return Gn(n)};Pe.hydrate=function(n,t,r){if(!Ds(t))throw Error(T(200));return Us(null,n,t,!0,r)};Pe.hydrateRoot=function(n,t,r){if(!ao(n))throw Error(T(405));var s=r!=null&&r.hydratedSources||null,l=!1,i="",o=Vd;if(r!=null&&(r.unstable_strictMode===!0&&(l=!0),r.identifierPrefix!==void 0&&(i=r.identifierPrefix),r.onRecoverableError!==void 0&&(o=r.onRecoverableError)),t=Wd(t,null,n,1,r??null,l,!1,i,o),n[on]=t.current,sr(n),s)for(n=0;n<s.length;n++)r=s[n],l=r._getVersion,l=l(r._source),t.mutableSourceEagerHydrationData==null?t.mutableSourceEagerHydrationData=[r,l]:t.mutableSourceEagerHydrationData.push(r,l);return new Fs(t)};Pe.render=function(n,t,r){if(!Ds(t))throw Error(T(200));return Us(null,n,t,!1,r)};Pe.unmountComponentAtNode=function(n){if(!Ds(n))throw Error(T(40));return n._reactRootContainer?(Gn(function(){Us(null,null,n,!1,function(){n._reactRootContainer=null,n[on]=null})}),!0):!1};Pe.unstable_batchedUpdates=no;Pe.unstable_renderSubtreeIntoContainer=function(n,t,r,s){if(!Ds(r))throw Error(T(200));if(n==null||n._reactInternals===void 0)throw Error(T(38));return Us(n,t,r,!1,s)};Pe.version="18.3.1-next-f1338f8080-20240426";function Yd(){if(!(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__>"u"||typeof __REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE!="function"))try{__REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE(Yd)}catch(n){console.error(n)}}Yd(),Ya.exports=Pe;var zm=Ya.exports,ba=zm;gl.createRoot=ba.createRoot,gl.hydrateRoot=ba.hydrateRoot;/**
 * @remix-run/router v1.23.1
 *
 * Copyright (c) Remix Software Inc.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE.md file in the root directory of this source tree.
 *
 * @license MIT
 */function mr(){return mr=Object.assign?Object.assign.bind():function(n){for(var t=1;t<arguments.length;t++){var r=arguments[t];for(var s in r)Object.prototype.hasOwnProperty.call(r,s)&&(n[s]=r[s])}return n},mr.apply(this,arguments)}var vn;(function(n){n.Pop="POP",n.Push="PUSH",n.Replace="REPLACE"})(vn||(vn={}));const wa="popstate";function $m(n){n===void 0&&(n={});function t(s,l){let{pathname:i,search:o,hash:a}=s.location;return ui("",{pathname:i,search:o,hash:a},l.state&&l.state.usr||null,l.state&&l.state.key||"default")}function r(s,l){return typeof l=="string"?l:bs(l)}return qm(t,r,null,n)}function re(n,t){if(n===!1||n===null||typeof n>"u")throw new Error(t)}function co(n,t){if(!n){typeof console<"u"&&console.warn(t);try{throw new Error(t)}catch{}}}function Hm(){return Math.random().toString(36).substr(2,8)}function Ea(n,t){return{usr:n.state,key:n.key,idx:t}}function ui(n,t,r,s){return r===void 0&&(r=null),mr({pathname:typeof n=="string"?n:n.pathname,search:"",hash:""},typeof t=="string"?Tt(t):t,{state:r,key:t&&t.key||s||Hm()})}function bs(n){let{pathname:t="/",search:r="",hash:s=""}=n;return r&&r!=="?"&&(t+=r.charAt(0)==="?"?r:"?"+r),s&&s!=="#"&&(t+=s.charAt(0)==="#"?s:"#"+s),t}function Tt(n){let t={};if(n){let r=n.indexOf("#");r>=0&&(t.hash=n.substr(r),n=n.substr(0,r));let s=n.indexOf("?");s>=0&&(t.search=n.substr(s),n=n.substr(0,s)),n&&(t.pathname=n)}return t}function qm(n,t,r,s){s===void 0&&(s={});let{window:l=document.defaultView,v5Compat:i=!1}=s,o=l.history,a=vn.Pop,c=null,u=x();u==null&&(u=0,o.replaceState(mr({},o.state,{idx:u}),""));function x(){return(o.state||{idx:null}).idx}function m(){a=vn.Pop;let w=x(),p=w==null?null:w-u;u=w,c&&c({action:a,location:g.location,delta:p})}function f(w,p){a=vn.Push;let d=ui(g.location,w,p);u=x()+1;let h=Ea(d,u),j=g.createHref(d);try{o.pushState(h,"",j)}catch(v){if(v instanceof DOMException&&v.name==="DataCloneError")throw v;l.location.assign(j)}i&&c&&c({action:a,location:g.location,delta:1})}function k(w,p){a=vn.Replace;let d=ui(g.location,w,p);u=x();let h=Ea(d,u),j=g.createHref(d);o.replaceState(h,"",j),i&&c&&c({action:a,location:g.location,delta:0})}function N(w){let p=l.location.origin!=="null"?l.location.origin:l.location.href,d=typeof w=="string"?w:bs(w);return d=d.replace(/ $/,"%20"),re(p,"No window.location.(origin|href) available to create URL for href: "+d),new URL(d,p)}let g={get action(){return a},get location(){return n(l,o)},listen(w){if(c)throw new Error("A history only accepts one active listener");return l.addEventListener(wa,m),c=w,()=>{l.removeEventListener(wa,m),c=null}},createHref(w){return t(l,w)},createURL:N,encodeLocation(w){let p=N(w);return{pathname:p.pathname,search:p.search,hash:p.hash}},push:f,replace:k,go(w){return o.go(w)}};return g}var Sa;(function(n){n.data="data",n.deferred="deferred",n.redirect="redirect",n.error="error"})(Sa||(Sa={}));function Gm(n,t,r){return r===void 0&&(r="/"),Wm(n,t,r)}function Wm(n,t,r,s){let l=typeof t=="string"?Tt(t):t,i=uo(l.pathname||"/",r);if(i==null)return null;let o=Qd(n);Vm(o);let a=null;for(let c=0;a==null&&c<o.length;++c){let u=ip(i);a=rp(o[c],u)}return a}function Qd(n,t,r,s){t===void 0&&(t=[]),r===void 0&&(r=[]),s===void 0&&(s="");let l=(i,o,a)=>{let c={relativePath:a===void 0?i.path||"":a,caseSensitive:i.caseSensitive===!0,childrenIndex:o,route:i};c.relativePath.startsWith("/")&&(re(c.relativePath.startsWith(s),'Absolute route path "'+c.relativePath+'" nested under path '+('"'+s+'" is not valid. An absolute child route path ')+"must start with the combined path of all its parent routes."),c.relativePath=c.relativePath.slice(s.length));let u=Rn([s,c.relativePath]),x=r.concat(c);i.children&&i.children.length>0&&(re(i.index!==!0,"Index routes must not have child routes. Please remove "+('all child routes from route path "'+u+'".')),Qd(i.children,t,x,u)),!(i.path==null&&!i.index)&&t.push({path:u,score:np(u,i.index),routesMeta:x})};return n.forEach((i,o)=>{var a;if(i.path===""||!((a=i.path)!=null&&a.includes("?")))l(i,o);else for(let c of Xd(i.path))l(i,o,c)}),t}function Xd(n){let t=n.split("/");if(t.length===0)return[];let[r,...s]=t,l=r.endsWith("?"),i=r.replace(/\?$/,"");if(s.length===0)return l?[i,""]:[i];let o=Xd(s.join("/")),a=[];return a.push(...o.map(c=>c===""?i:[i,c].join("/"))),l&&a.push(...o),a.map(c=>n.startsWith("/")&&c===""?"/":c)}function Vm(n){n.sort((t,r)=>t.score!==r.score?r.score-t.score:tp(t.routesMeta.map(s=>s.childrenIndex),r.routesMeta.map(s=>s.childrenIndex)))}const Ym=/^:[\w-]+$/,Qm=3,Xm=2,Zm=1,Jm=10,ep=-2,Ta=n=>n==="*";function np(n,t){let r=n.split("/"),s=r.length;return r.some(Ta)&&(s+=ep),t&&(s+=Xm),r.filter(l=>!Ta(l)).reduce((l,i)=>l+(Ym.test(i)?Qm:i===""?Zm:Jm),s)}function tp(n,t){return n.length===t.length&&n.slice(0,-1).every((s,l)=>s===t[l])?n[n.length-1]-t[t.length-1]:0}function rp(n,t,r){let{routesMeta:s}=n,l={},i="/",o=[];for(let a=0;a<s.length;++a){let c=s[a],u=a===s.length-1,x=i==="/"?t:t.slice(i.length)||"/",m=sp({path:c.relativePath,caseSensitive:c.caseSensitive,end:u},x),f=c.route;if(!m)return null;Object.assign(l,m.params),o.push({params:l,pathname:Rn([i,m.pathname]),pathnameBase:up(Rn([i,m.pathnameBase])),route:f}),m.pathnameBase!=="/"&&(i=Rn([i,m.pathnameBase]))}return o}function sp(n,t){typeof n=="string"&&(n={path:n,caseSensitive:!1,end:!0});let[r,s]=lp(n.path,n.caseSensitive,n.end),l=t.match(r);if(!l)return null;let i=l[0],o=i.replace(/(.)\/+$/,"$1"),a=l.slice(1);return{params:s.reduce((u,x,m)=>{let{paramName:f,isOptional:k}=x;if(f==="*"){let g=a[m]||"";o=i.slice(0,i.length-g.length).replace(/(.)\/+$/,"$1")}const N=a[m];return k&&!N?u[f]=void 0:u[f]=(N||"").replace(/%2F/g,"/"),u},{}),pathname:i,pathnameBase:o,pattern:n}}function lp(n,t,r){t===void 0&&(t=!1),r===void 0&&(r=!0),co(n==="*"||!n.endsWith("*")||n.endsWith("/*"),'Route path "'+n+'" will be treated as if it were '+('"'+n.replace(/\*$/,"/*")+'" because the `*` character must ')+"always follow a `/` in the pattern. To get rid of this warning, "+('please change the route path to "'+n.replace(/\*$/,"/*")+'".'));let s=[],l="^"+n.replace(/\/*\*?$/,"").replace(/^\/*/,"/").replace(/[\\.*+^${}|()[\]]/g,"\\$&").replace(/\/:([\w-]+)(\?)?/g,(o,a,c)=>(s.push({paramName:a,isOptional:c!=null}),c?"/?([^\\/]+)?":"/([^\\/]+)"));return n.endsWith("*")?(s.push({paramName:"*"}),l+=n==="*"||n==="/*"?"(.*)$":"(?:\\/(.+)|\\/*)$"):r?l+="\\/*$":n!==""&&n!=="/"&&(l+="(?:(?=\\/|$))"),[new RegExp(l,t?void 0:"i"),s]}function ip(n){try{return n.split("/").map(t=>decodeURIComponent(t).replace(/\//g,"%2F")).join("/")}catch(t){return co(!1,'The URL path "'+n+'" could not be decoded because it is is a malformed URL segment. This is probably due to a bad percent '+("encoding ("+t+").")),n}}function uo(n,t){if(t==="/")return n;if(!n.toLowerCase().startsWith(t.toLowerCase()))return null;let r=t.endsWith("/")?t.length-1:t.length,s=n.charAt(r);return s&&s!=="/"?null:n.slice(r)||"/"}const op=/^(?:[a-z][a-z0-9+.-]*:|\/\/)/i,ap=n=>op.test(n);function cp(n,t){t===void 0&&(t="/");let{pathname:r,search:s="",hash:l=""}=typeof n=="string"?Tt(n):n,i;if(r)if(ap(r))i=r;else{if(r.includes("//")){let o=r;r=r.replace(/\/\/+/g,"/"),co(!1,"Pathnames cannot have embedded double slashes - normalizing "+(o+" -> "+r))}r.startsWith("/")?i=Ra(r.substring(1),"/"):i=Ra(r,t)}else i=t;return{pathname:i,search:hp(s),hash:mp(l)}}function Ra(n,t){let r=t.replace(/\/+$/,"").split("/");return n.split("/").forEach(l=>{l===".."?r.length>1&&r.pop():l!=="."&&r.push(l)}),r.length>1?r.join("/"):"/"}function xl(n,t,r,s){return"Cannot include a '"+n+"' character in a manually specified "+("`to."+t+"` field ["+JSON.stringify(s)+"].  Please separate it out to the ")+("`to."+r+"` field. Alternatively you may provide the full path as ")+'a string in <Link to="..."> and the router will parse it for you.'}function dp(n){return n.filter((t,r)=>r===0||t.route.path&&t.route.path.length>0)}function Zd(n,t){let r=dp(n);return t?r.map((s,l)=>l===r.length-1?s.pathname:s.pathnameBase):r.map(s=>s.pathnameBase)}function Jd(n,t,r,s){s===void 0&&(s=!1);let l;typeof n=="string"?l=Tt(n):(l=mr({},n),re(!l.pathname||!l.pathname.includes("?"),xl("?","pathname","search",l)),re(!l.pathname||!l.pathname.includes("#"),xl("#","pathname","hash",l)),re(!l.search||!l.search.includes("#"),xl("#","search","hash",l)));let i=n===""||l.pathname==="",o=i?"/":l.pathname,a;if(o==null)a=r;else{let m=t.length-1;if(!s&&o.startsWith("..")){let f=o.split("/");for(;f[0]==="..";)f.shift(),m-=1;l.pathname=f.join("/")}a=m>=0?t[m]:"/"}let c=cp(l,a),u=o&&o!=="/"&&o.endsWith("/"),x=(i||o===".")&&r.endsWith("/");return!c.pathname.endsWith("/")&&(u||x)&&(c.pathname+="/"),c}const Rn=n=>n.join("/").replace(/\/\/+/g,"/"),up=n=>n.replace(/\/+$/,"").replace(/^\/*/,"/"),hp=n=>!n||n==="?"?"":n.startsWith("?")?n:"?"+n,mp=n=>!n||n==="#"?"":n.startsWith("#")?n:"#"+n;function pp(n){return n!=null&&typeof n.status=="number"&&typeof n.statusText=="string"&&typeof n.internal=="boolean"&&"data"in n}const eu=["post","put","patch","delete"];new Set(eu);const fp=["get",...eu];new Set(fp);/**
 * React Router v6.30.2
 *
 * Copyright (c) Remix Software Inc.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE.md file in the root directory of this source tree.
 *
 * @license MIT
 */function pr(){return pr=Object.assign?Object.assign.bind():function(n){for(var t=1;t<arguments.length;t++){var r=arguments[t];for(var s in r)Object.prototype.hasOwnProperty.call(r,s)&&(n[s]=r[s])}return n},pr.apply(this,arguments)}const ho=y.createContext(null),xp=y.createContext(null),Yn=y.createContext(null),Ks=y.createContext(null),Qn=y.createContext({outlet:null,matches:[],isDataRoute:!1}),nu=y.createContext(null);function jp(n,t){let{relative:r}=t===void 0?{}:t;yr()||re(!1);let{basename:s,navigator:l}=y.useContext(Yn),{hash:i,pathname:o,search:a}=ru(n,{relative:r}),c=o;return s!=="/"&&(c=o==="/"?s:Rn([s,o])),l.createHref({pathname:c,search:a,hash:i})}function yr(){return y.useContext(Ks)!=null}function kr(){return yr()||re(!1),y.useContext(Ks).location}function tu(n){y.useContext(Yn).static||y.useLayoutEffect(n)}function gp(){let{isDataRoute:n}=y.useContext(Qn);return n?Pp():vp()}function vp(){yr()||re(!1);let n=y.useContext(ho),{basename:t,future:r,navigator:s}=y.useContext(Yn),{matches:l}=y.useContext(Qn),{pathname:i}=kr(),o=JSON.stringify(Zd(l,r.v7_relativeSplatPath)),a=y.useRef(!1);return tu(()=>{a.current=!0}),y.useCallback(function(u,x){if(x===void 0&&(x={}),!a.current)return;if(typeof u=="number"){s.go(u);return}let m=Jd(u,JSON.parse(o),i,x.relative==="path");n==null&&t!=="/"&&(m.pathname=m.pathname==="/"?t:Rn([t,m.pathname])),(x.replace?s.replace:s.push)(m,x.state,x)},[t,s,o,i,n])}function ru(n,t){let{relative:r}=t===void 0?{}:t,{future:s}=y.useContext(Yn),{matches:l}=y.useContext(Qn),{pathname:i}=kr(),o=JSON.stringify(Zd(l,s.v7_relativeSplatPath));return y.useMemo(()=>Jd(n,JSON.parse(o),i,r==="path"),[n,o,i,r])}function yp(n,t){return kp(n,t)}function kp(n,t,r,s){yr()||re(!1);let{navigator:l}=y.useContext(Yn),{matches:i}=y.useContext(Qn),o=i[i.length-1],a=o?o.params:{};o&&o.pathname;let c=o?o.pathnameBase:"/";o&&o.route;let u=kr(),x;if(t){var m;let w=typeof t=="string"?Tt(t):t;c==="/"||(m=w.pathname)!=null&&m.startsWith(c)||re(!1),x=w}else x=u;let f=x.pathname||"/",k=f;if(c!=="/"){let w=c.replace(/^\//,"").split("/");k="/"+f.replace(/^\//,"").split("/").slice(w.length).join("/")}let N=Gm(n,{pathname:k}),g=Sp(N&&N.map(w=>Object.assign({},w,{params:Object.assign({},a,w.params),pathname:Rn([c,l.encodeLocation?l.encodeLocation(w.pathname).pathname:w.pathname]),pathnameBase:w.pathnameBase==="/"?c:Rn([c,l.encodeLocation?l.encodeLocation(w.pathnameBase).pathname:w.pathnameBase])})),i,r,s);return t&&g?y.createElement(Ks.Provider,{value:{location:pr({pathname:"/",search:"",hash:"",state:null,key:"default"},x),navigationType:vn.Pop}},g):g}function Np(){let n=_p(),t=pp(n)?n.status+" "+n.statusText:n instanceof Error?n.message:JSON.stringify(n),r=n instanceof Error?n.stack:null,l={padding:"0.5rem",backgroundColor:"rgba(200,200,200, 0.5)"};return y.createElement(y.Fragment,null,y.createElement("h2",null,"Unexpected Application Error!"),y.createElement("h3",{style:{fontStyle:"italic"}},t),r?y.createElement("pre",{style:l},r):null,null)}const bp=y.createElement(Np,null);class wp extends y.Component{constructor(t){super(t),this.state={location:t.location,revalidation:t.revalidation,error:t.error}}static getDerivedStateFromError(t){return{error:t}}static getDerivedStateFromProps(t,r){return r.location!==t.location||r.revalidation!=="idle"&&t.revalidation==="idle"?{error:t.error,location:t.location,revalidation:t.revalidation}:{error:t.error!==void 0?t.error:r.error,location:r.location,revalidation:t.revalidation||r.revalidation}}componentDidCatch(t,r){console.error("React Router caught the following error during render",t,r)}render(){return this.state.error!==void 0?y.createElement(Qn.Provider,{value:this.props.routeContext},y.createElement(nu.Provider,{value:this.state.error,children:this.props.component})):this.props.children}}function Ep(n){let{routeContext:t,match:r,children:s}=n,l=y.useContext(ho);return l&&l.static&&l.staticContext&&(r.route.errorElement||r.route.ErrorBoundary)&&(l.staticContext._deepestRenderedBoundaryId=r.route.id),y.createElement(Qn.Provider,{value:t},s)}function Sp(n,t,r,s){var l;if(t===void 0&&(t=[]),r===void 0&&(r=null),s===void 0&&(s=null),n==null){var i;if(!r)return null;if(r.errors)n=r.matches;else if((i=s)!=null&&i.v7_partialHydration&&t.length===0&&!r.initialized&&r.matches.length>0)n=r.matches;else return null}let o=n,a=(l=r)==null?void 0:l.errors;if(a!=null){let x=o.findIndex(m=>m.route.id&&(a==null?void 0:a[m.route.id])!==void 0);x>=0||re(!1),o=o.slice(0,Math.min(o.length,x+1))}let c=!1,u=-1;if(r&&s&&s.v7_partialHydration)for(let x=0;x<o.length;x++){let m=o[x];if((m.route.HydrateFallback||m.route.hydrateFallbackElement)&&(u=x),m.route.id){let{loaderData:f,errors:k}=r,N=m.route.loader&&f[m.route.id]===void 0&&(!k||k[m.route.id]===void 0);if(m.route.lazy||N){c=!0,u>=0?o=o.slice(0,u+1):o=[o[0]];break}}}return o.reduceRight((x,m,f)=>{let k,N=!1,g=null,w=null;r&&(k=a&&m.route.id?a[m.route.id]:void 0,g=m.route.errorElement||bp,c&&(u<0&&f===0?(Ap("route-fallback"),N=!0,w=null):u===f&&(N=!0,w=m.route.hydrateFallbackElement||null)));let p=t.concat(o.slice(0,f+1)),d=()=>{let h;return k?h=g:N?h=w:m.route.Component?h=y.createElement(m.route.Component,null):m.route.element?h=m.route.element:h=x,y.createElement(Ep,{match:m,routeContext:{outlet:x,matches:p,isDataRoute:r!=null},children:h})};return r&&(m.route.ErrorBoundary||m.route.errorElement||f===0)?y.createElement(wp,{location:r.location,revalidation:r.revalidation,component:g,error:k,children:d(),routeContext:{outlet:null,matches:p,isDataRoute:!0}}):d()},null)}var su=function(n){return n.UseBlocker="useBlocker",n.UseRevalidator="useRevalidator",n.UseNavigateStable="useNavigate",n}(su||{}),lu=function(n){return n.UseBlocker="useBlocker",n.UseLoaderData="useLoaderData",n.UseActionData="useActionData",n.UseRouteError="useRouteError",n.UseNavigation="useNavigation",n.UseRouteLoaderData="useRouteLoaderData",n.UseMatches="useMatches",n.UseRevalidator="useRevalidator",n.UseNavigateStable="useNavigate",n.UseRouteId="useRouteId",n}(lu||{});function Tp(n){let t=y.useContext(ho);return t||re(!1),t}function Rp(n){let t=y.useContext(xp);return t||re(!1),t}function Cp(n){let t=y.useContext(Qn);return t||re(!1),t}function iu(n){let t=Cp(),r=t.matches[t.matches.length-1];return r.route.id||re(!1),r.route.id}function _p(){var n;let t=y.useContext(nu),r=Rp(),s=iu();return t!==void 0?t:(n=r.errors)==null?void 0:n[s]}function Pp(){let{router:n}=Tp(su.UseNavigateStable),t=iu(lu.UseNavigateStable),r=y.useRef(!1);return tu(()=>{r.current=!0}),y.useCallback(function(l,i){i===void 0&&(i={}),r.current&&(typeof l=="number"?n.navigate(l):n.navigate(l,pr({fromRouteId:t},i)))},[n,t])}const Ca={};function Ap(n,t,r){Ca[n]||(Ca[n]=!0)}function Ip(n,t){n==null||n.v7_startTransition,n==null||n.v7_relativeSplatPath}function $(n){re(!1)}function Op(n){let{basename:t="/",children:r=null,location:s,navigationType:l=vn.Pop,navigator:i,static:o=!1,future:a}=n;yr()&&re(!1);let c=t.replace(/^\/*/,"/"),u=y.useMemo(()=>({basename:c,navigator:i,static:o,future:pr({v7_relativeSplatPath:!1},a)}),[c,a,i,o]);typeof s=="string"&&(s=Tt(s));let{pathname:x="/",search:m="",hash:f="",state:k=null,key:N="default"}=s,g=y.useMemo(()=>{let w=uo(x,c);return w==null?null:{location:{pathname:w,search:m,hash:f,state:k,key:N},navigationType:l}},[c,x,m,f,k,N,l]);return g==null?null:y.createElement(Yn.Provider,{value:u},y.createElement(Ks.Provider,{children:r,value:g}))}function Mp(n){let{children:t,location:r}=n;return yp(hi(t),r)}new Promise(()=>{});function hi(n,t){t===void 0&&(t=[]);let r=[];return y.Children.forEach(n,(s,l)=>{if(!y.isValidElement(s))return;let i=[...t,l];if(s.type===y.Fragment){r.push.apply(r,hi(s.props.children,i));return}s.type!==$&&re(!1),!s.props.index||!s.props.children||re(!1);let o={id:s.props.id||i.join("-"),caseSensitive:s.props.caseSensitive,element:s.props.element,Component:s.props.Component,index:s.props.index,path:s.props.path,loader:s.props.loader,action:s.props.action,errorElement:s.props.errorElement,ErrorBoundary:s.props.ErrorBoundary,hasErrorBoundary:s.props.ErrorBoundary!=null||s.props.errorElement!=null,shouldRevalidate:s.props.shouldRevalidate,handle:s.props.handle,lazy:s.props.lazy};s.props.children&&(o.children=hi(s.props.children,i)),r.push(o)}),r}/**
 * React Router DOM v6.30.2
 *
 * Copyright (c) Remix Software Inc.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE.md file in the root directory of this source tree.
 *
 * @license MIT
 */function mi(){return mi=Object.assign?Object.assign.bind():function(n){for(var t=1;t<arguments.length;t++){var r=arguments[t];for(var s in r)Object.prototype.hasOwnProperty.call(r,s)&&(n[s]=r[s])}return n},mi.apply(this,arguments)}function Lp(n,t){if(n==null)return{};var r={},s=Object.keys(n),l,i;for(i=0;i<s.length;i++)l=s[i],!(t.indexOf(l)>=0)&&(r[l]=n[l]);return r}function Fp(n){return!!(n.metaKey||n.altKey||n.ctrlKey||n.shiftKey)}function Dp(n,t){return n.button===0&&(!t||t==="_self")&&!Fp(n)}const Up=["onClick","relative","reloadDocument","replace","state","target","to","preventScrollReset","viewTransition"],Kp="6";try{window.__reactRouterVersion=Kp}catch{}const Bp="startTransition",_a=Pu[Bp];function zp(n){let{basename:t,children:r,future:s,window:l}=n,i=y.useRef();i.current==null&&(i.current=$m({window:l,v5Compat:!0}));let o=i.current,[a,c]=y.useState({action:o.action,location:o.location}),{v7_startTransition:u}=s||{},x=y.useCallback(m=>{u&&_a?_a(()=>c(m)):c(m)},[c,u]);return y.useLayoutEffect(()=>o.listen(x),[o,x]),y.useEffect(()=>Ip(s),[s]),y.createElement(Op,{basename:t,children:r,location:a.location,navigationType:a.action,navigator:o,future:s})}const $p=typeof window<"u"&&typeof window.document<"u"&&typeof window.document.createElement<"u",Hp=/^(?:[a-z][a-z0-9+.-]*:|\/\/)/i,Bn=y.forwardRef(function(t,r){let{onClick:s,relative:l,reloadDocument:i,replace:o,state:a,target:c,to:u,preventScrollReset:x,viewTransition:m}=t,f=Lp(t,Up),{basename:k}=y.useContext(Yn),N,g=!1;if(typeof u=="string"&&Hp.test(u)&&(N=u,$p))try{let h=new URL(window.location.href),j=u.startsWith("//")?new URL(h.protocol+u):new URL(u),v=uo(j.pathname,k);j.origin===h.origin&&v!=null?u=v+j.search+j.hash:g=!0}catch{}let w=jp(u,{relative:l}),p=qp(u,{replace:o,state:a,target:c,preventScrollReset:x,relative:l,viewTransition:m});function d(h){s&&s(h),h.defaultPrevented||p(h)}return y.createElement("a",mi({},f,{href:N||w,onClick:g||i?s:d,ref:r,target:c}))});var Pa;(function(n){n.UseScrollRestoration="useScrollRestoration",n.UseSubmit="useSubmit",n.UseSubmitFetcher="useSubmitFetcher",n.UseFetcher="useFetcher",n.useViewTransitionState="useViewTransitionState"})(Pa||(Pa={}));var Aa;(function(n){n.UseFetcher="useFetcher",n.UseFetchers="useFetchers",n.UseScrollRestoration="useScrollRestoration"})(Aa||(Aa={}));function qp(n,t){let{target:r,replace:s,state:l,preventScrollReset:i,relative:o,viewTransition:a}=t===void 0?{}:t,c=gp(),u=kr(),x=ru(n,{relative:o});return y.useCallback(m=>{if(Dp(m,r)){m.preventDefault();let f=s!==void 0?s:bs(u)===bs(x);c(n,{replace:f,state:l,preventScrollReset:i,relative:o,viewTransition:a})}},[u,c,x,s,l,r,n,i,o,a])}class Gp{constructor(){mo(this,"baseUrl","/v1")}async request(t,r={}){var l;const s=await fetch(`${this.baseUrl}${t}`,{...r,headers:{"Content-Type":"application/json",...r.headers}});if(!s.ok){const i=await s.json();throw new Error(((l=i.error)==null?void 0:l.message)||`HTTP ${s.status}`)}if(s.status!==204)return s.json()}async listModels(){return this.request("/models")}async rebuildModelIndex(){var r;const t=await fetch(`${this.baseUrl}/models/index`,{method:"POST"});if(!t.ok){const s=await t.json();throw new Error(((r=s.error)==null?void 0:r.message)||`HTTP ${t.status}`)}}async listRunningModels(){return this.request("/models/ps")}async showModel(t){return this.request(`/models/${encodeURIComponent(t)}`)}async getLibsVersion(){return this.request("/libs")}async pullModelAsync(t){var s;const r=await fetch(`${this.baseUrl}/models/pull`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({model_url:t,async:!0})});if(!r.ok){const l=await r.json();throw new Error(((s=l.error)==null?void 0:s.message)||`HTTP ${r.status}`)}return r.json()}streamPullSession(t,r,s,l){const i=new AbortController;return fetch(`${this.baseUrl}/models/pull/${encodeURIComponent(t)}`,{method:"GET",signal:i.signal}).then(async o=>{var x;if(o.status===400){s("Session closed");return}if(!o.ok){s(`HTTP ${o.status}`);return}const a=(x=o.body)==null?void 0:x.getReader();if(!a){s("Streaming not supported");return}const c=new TextDecoder;let u="";for(;;){const{done:m,value:f}=await a.read();if(m)break;u+=c.decode(f,{stream:!0});const k=u.split(`
`);u=k.pop()||"";for(const N of k){if(!N.trim())continue;const g=N.startsWith("data: ")?N.slice(6):N;if(g.trim())try{const w=JSON.parse(g);if(r(w),w.status==="downloaded"||w.downloaded){l();return}}catch{s("Failed to parse response")}}}l()}).catch(o=>{o.name!=="AbortError"&&s("Connection error")}),()=>i.abort()}pullModel(t,r,s,l){const i=new AbortController;return fetch(`${this.baseUrl}/models/pull`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({model_url:t}),signal:i.signal}).then(async o=>{var x;if(!o.ok){s(`HTTP ${o.status}`);return}const a=(x=o.body)==null?void 0:x.getReader();if(!a){s("Streaming not supported");return}const c=new TextDecoder;let u="";for(;;){const{done:m,value:f}=await a.read();if(m)break;u+=c.decode(f,{stream:!0});const k=u.split(`
`);u=k.pop()||"";for(const N of k){if(!N.trim())continue;const g=N.startsWith("data: ")?N.slice(6):N;if(g.trim())try{const w=JSON.parse(g);if(r(w),w.status==="complete"||w.downloaded){l();return}}catch{s("Failed to parse response")}}}l()}).catch(o=>{o.name!=="AbortError"&&s("Connection error")}),()=>i.abort()}async removeModel(t){await this.request(`/models/${encodeURIComponent(t)}`,{method:"DELETE"})}async listCatalog(){return this.request("/catalog")}async showCatalogModel(t){return this.request(`/catalog/${encodeURIComponent(t)}`)}pullCatalogModel(t,r,s,l){const i=new AbortController;return fetch(`${this.baseUrl}/catalog/pull/${encodeURIComponent(t)}`,{method:"POST",signal:i.signal}).then(async o=>{var x;if(!o.ok){s(`HTTP ${o.status}`);return}const a=(x=o.body)==null?void 0:x.getReader();if(!a){s("Streaming not supported");return}const c=new TextDecoder;let u="";for(;;){const{done:m,value:f}=await a.read();if(m)break;u+=c.decode(f,{stream:!0});const k=u.split(`
`);u=k.pop()||"";for(const N of k){if(!N.trim())continue;const g=N.startsWith("data: ")?N.slice(6):N;if(g.trim())try{const w=JSON.parse(g);if(r(w),w.status==="complete"||w.downloaded){l();return}}catch{s("Failed to parse response")}}}l()}).catch(o=>{o.name!=="AbortError"&&s("Connection error")}),()=>i.abort()}pullLibs(t,r,s){const l=new AbortController;return fetch(`${this.baseUrl}/libs/pull`,{method:"POST",signal:l.signal}).then(async i=>{var u;if(!i.ok){r(`HTTP ${i.status}`);return}const o=(u=i.body)==null?void 0:u.getReader();if(!o){r("Streaming not supported");return}const a=new TextDecoder;let c="";for(;;){const{done:x,value:m}=await o.read();if(x)break;c+=a.decode(m,{stream:!0});const f=c.split(`
`);c=f.pop()||"";for(const k of f){if(!k.trim())continue;const N=k.startsWith("data: ")?k.slice(6):k;if(N.trim())try{const g=JSON.parse(N);if(t(g),g.status==="complete"){s();return}}catch{r("Failed to parse response")}}}s()}).catch(i=>{i.name!=="AbortError"&&r("Connection error")}),()=>l.abort()}async listKeys(t){return this.request("/security/keys",{headers:{Authorization:`Bearer ${t}`}})}async createKey(t){return this.request("/security/keys/add",{method:"POST",headers:{Authorization:`Bearer ${t}`}})}async deleteKey(t,r){await this.request(`/security/keys/remove/${encodeURIComponent(r)}`,{method:"POST",headers:{Authorization:`Bearer ${t}`}})}async createToken(t,r){return this.request("/security/token/create",{method:"POST",headers:{Authorization:`Bearer ${t}`},body:JSON.stringify(r)})}streamChat(t,r,s,l){const i=new AbortController;return fetch(`${this.baseUrl}/chat/completions`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({...t,stream:!0}),signal:i.signal}).then(async o=>{var x,m;if(!o.ok){const f=await o.json();s(((x=f.error)==null?void 0:x.message)||`HTTP ${o.status}`);return}const a=(m=o.body)==null?void 0:m.getReader();if(!a){s("Streaming not supported");return}const c=new TextDecoder;let u="";for(;;){const{done:f,value:k}=await a.read();if(f)break;u+=c.decode(k,{stream:!0});const N=u.split(`
`);u=N.pop()||"";for(const g of N){if(!g.trim()||g==="data: [DONE]")continue;const w=g.startsWith("data: ")?g.slice(6):g;if(w.trim())try{const p=JSON.parse(w);r(p)}catch{}}}l()}).catch(o=>{o.name!=="AbortError"&&s(o.message||"Connection error")}),()=>i.abort()}}const fe=new Gp,ou=y.createContext(null);function Wp({children:n}){const[t,r]=y.useState(null),[s,l]=y.useState(!1),[i,o]=y.useState(null),[a,c]=y.useState(!1),u=y.useCallback(async()=>{if(!(a&&t)){l(!0),o(null);try{const m=await fe.listModels();r(m),c(!0)}catch(m){o(m instanceof Error?m.message:"Failed to load models")}finally{l(!1)}}},[a,t]),x=y.useCallback(()=>{c(!1),r(null)},[]);return e.jsx(ou.Provider,{value:{models:t,loading:s,error:i,loadModels:u,invalidate:x},children:n})}function Bs(){const n=y.useContext(ou);if(!n)throw new Error("useModelList must be used within a ModelListProvider");return n}const au=y.createContext(null);function Vp({children:n}){const{invalidate:t}=Bs(),[r,s]=y.useState(null),l=y.useRef(null),i="\r\x1B[K",o=y.useCallback((f,k)=>{s(N=>N&&{...N,messages:[...N.messages,{text:f,type:k}]})},[]),a=y.useCallback((f,k)=>{s(N=>{if(!N)return N;if(N.messages.length===0)return{...N,messages:[{text:f,type:k}]};const g=[...N.messages];return g[g.length-1]={text:f,type:k},{...N,messages:g}})},[]),c=y.useCallback(f=>{l.current||(s({modelUrl:f,messages:[],status:"downloading"}),l.current=fe.pullModel(f,k=>{if(k.status)if(k.status.startsWith(i)){const N=k.status.slice(i.length);a(N,"info")}else o(k.status,"info");k.model_file&&o(`Model file: ${k.model_file}`,"info")},k=>{o(k,"error"),s(N=>N&&{...N,status:"error"}),l.current=null},()=>{o("Pull complete!","success"),s(k=>k&&{...k,status:"complete"}),l.current=null,t()}))},[o,a,t]),u=y.useCallback(()=>{l.current&&(l.current(),l.current=null),s(f=>f&&{...f,messages:[...f.messages,{text:"Cancelled",type:"error"}],status:"error"})},[]),x=y.useCallback(()=>{l.current&&(l.current(),l.current=null),s(null)},[]),m=(r==null?void 0:r.status)==="downloading";return e.jsx(au.Provider,{value:{download:r,isDownloading:m,startDownload:c,cancelDownload:u,clearDownload:x},children:n})}function cu(){const n=y.useContext(au);if(!n)throw new Error("useDownload must be used within a DownloadProvider");return n}const Yp=[{id:"settings",label:"Settings",items:[{page:"settings",label:"API Token"}]},{id:"model",label:"Models",items:[{page:"model-list",label:"List"},{page:"model-ps",label:"Running"},{page:"model-pull",label:"Pull"}]},{id:"catalog",label:"Catalog",items:[{page:"catalog-list",label:"List"}]},{id:"libs",label:"Libs",items:[{page:"libs-pull",label:"Pull"}]},{id:"security",label:"Security",subcategories:[{id:"security-key",label:"Key",items:[{page:"security-key-list",label:"List"},{page:"security-key-create",label:"Create"},{page:"security-key-delete",label:"Delete"}]},{id:"security-token",label:"Token",items:[{page:"security-token-create",label:"Create"}]}]},{id:"docs",label:"Docs",subcategories:[{id:"docs-sdk",label:"SDK",items:[{page:"docs-sdk-kronk",label:"Kronk"},{page:"docs-sdk-model",label:"Model"},{page:"docs-sdk-examples",label:"Examples"}]},{id:"docs-cli-sub",label:"CLI",items:[{page:"docs-cli-catalog",label:"catalog"},{page:"docs-cli-libs",label:"libs"},{page:"docs-cli-model",label:"model"},{page:"docs-cli-run",label:"run"},{page:"docs-cli-security",label:"security"},{page:"docs-cli-server",label:"server"}]},{id:"docs-api-sub",label:"Web API",items:[{page:"docs-api-chat",label:"Chat"},{page:"docs-api-messages",label:"Messages"},{page:"docs-api-responses",label:"Responses"},{page:"docs-api-embeddings",label:"Embeddings"},{page:"docs-api-rerank",label:"Rerank"},{page:"docs-api-tools",label:"Tools"}]}]},{id:"run",label:"Run",items:[{page:"chat",label:"Chat"}]}];function Qp({children:n}){const t=kr(),r=Bf[t.pathname]||"home",[s,l]=y.useState(new Set),{download:i,isDownloading:o}=cu(),a=m=>{l(f=>{const k=new Set(f);return k.has(m)?k.delete(m):k.add(m),k})},c=m=>m.items?m.items.some(f=>f.page===r):m.subcategories?m.subcategories.some(f=>c(f)):!1,u=m=>e.jsx(Bn,{to:pi[m.page],className:`menu-item ${r===m.page?"active":""}`,children:m.label},m.page),x=(m,f=!1)=>{var g,w;const k=s.has(m.id),N=c(m);return e.jsxs("div",{className:`menu-category ${f?"submenu":""}`,children:[e.jsxs("div",{className:`menu-category-header ${N?"active":""}`,onClick:()=>a(m.id),children:[e.jsx("span",{children:m.label}),e.jsx("span",{className:`menu-category-arrow ${k?"expanded":""}`,children:"▶"})]}),e.jsxs("div",{className:`menu-items ${k?"expanded":""}`,children:[(g=m.subcategories)==null?void 0:g.map(p=>x(p,!0)),(w=m.items)==null?void 0:w.map(u)]})]},m.id)};return e.jsxs("div",{className:"app",children:[e.jsxs("aside",{className:"sidebar",children:[e.jsx("div",{className:"sidebar-header",children:e.jsxs(Bn,{to:"/",style:{textDecoration:"none",color:"inherit"},className:"sidebar-brand",children:[e.jsx("img",{src:"/kronk-logo.png",alt:"Kronk Logo",className:"sidebar-logo"}),e.jsx("h1",{children:"Model Server"})]})}),e.jsx("nav",{children:Yp.map(m=>x(m))}),i&&e.jsx("div",{className:"download-indicator",children:e.jsxs(Bn,{to:pi["model-pull"],className:"download-indicator-link",children:[e.jsxs("div",{className:"download-indicator-header",children:[o?e.jsx("span",{className:"download-indicator-spinner"}):i.status==="complete"?e.jsx("span",{className:"download-indicator-icon success",children:"✓"}):e.jsx("span",{className:"download-indicator-icon error",children:"✗"}),e.jsx("span",{className:"download-indicator-title",children:o?"Downloading...":i.status==="complete"?"Complete":"Failed"})]}),e.jsx("div",{className:"download-indicator-url",title:i.modelUrl,children:i.modelUrl.split("/").pop()})]})})]}),e.jsx("main",{className:"main-content",children:n})]})}function Ia(n){if(n===0)return"0 B";const t=1024,r=["B","KB","MB","GB","TB"],s=Math.floor(Math.log(n)/Math.log(t));return parseFloat((n/Math.pow(t,s)).toFixed(2))+" "+r[s]}function Xp(n){return new Date(n).toLocaleString()}function Zp(){const{models:n,loading:t,error:r,loadModels:s,invalidate:l}=Bs(),[i,o]=y.useState(null),[a,c]=y.useState(null),[u,x]=y.useState(!1),[m,f]=y.useState(null),[k,N]=y.useState(!1),[g,w]=y.useState(null),[p,d]=y.useState(!1),[h,j]=y.useState(!1),[v,b]=y.useState(!1),[E,S]=y.useState(null),[C,_]=y.useState(null),U=async()=>{N(!0),w(null),d(!1);try{await fe.rebuildModelIndex(),l(),s(),o(null),c(null),d(!0),setTimeout(()=>d(!1),3e3)}catch(L){w(L instanceof Error?L.message:"Failed to rebuild index")}finally{N(!1)}};y.useEffect(()=>{s()},[s]);const A=async L=>{if(i===L){o(null),c(null),j(!1);return}o(L),j(!1),S(null),_(null),x(!0),f(null),c(null);try{const G=await fe.showModel(L);c(G)}catch(G){f(G instanceof Error?G.message:"Failed to load model info")}finally{x(!1)}},z=()=>{i&&j(!0)},Ie=async()=>{if(i){b(!0),j(!1),S(null),_(null);try{await fe.removeModel(i),_(`Model "${i}" removed successfully`),o(null),c(null),l(),await s(),setTimeout(()=>_(null),3e3)}catch(L){S(L instanceof Error?L.message:"Failed to remove model")}finally{b(!1)}}},I=()=>{j(!1)};return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Models"}),e.jsx("p",{children:"List of all models available in the system. Click a model to view details."})]}),e.jsxs("div",{className:"card",children:[t&&e.jsx("div",{className:"loading",children:"Loading models"}),r&&e.jsx("div",{className:"alert alert-error",children:r}),E&&e.jsx("div",{className:"alert alert-error",children:E}),C&&e.jsx("div",{className:"alert alert-success",children:C}),!t&&!r&&n&&e.jsx("div",{className:"table-container",children:n.data&&n.data.length>0?e.jsxs("table",{children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{style:{width:"40px",textAlign:"center"},title:"Validated",children:"✓"}),e.jsx("th",{children:"ID"}),e.jsx("th",{children:"Owner"}),e.jsx("th",{children:"Family"}),e.jsx("th",{children:"Size"}),e.jsx("th",{children:"Modified"})]})}),e.jsx("tbody",{children:n.data.map(L=>e.jsxs("tr",{onClick:()=>A(L.id),className:i===L.id?"selected":"",style:{cursor:"pointer"},children:[e.jsx("td",{style:{textAlign:"center",color:L.validated?"inherit":"#e74c3c"},children:L.validated?"✓":"✗"}),e.jsx("td",{children:L.id}),e.jsx("td",{children:L.owned_by}),e.jsx("td",{children:L.model_family}),e.jsx("td",{children:Ia(L.size)}),e.jsx("td",{children:Xp(L.modified)})]},L.id))})]}):e.jsxs("div",{className:"empty-state",children:[e.jsx("h3",{children:"No models found"}),e.jsx("p",{children:"Pull a model to get started"})]})}),e.jsxs("div",{style:{marginTop:"16px",display:"flex",gap:"8px"},children:[e.jsx("button",{className:"btn btn-secondary",onClick:()=>{l(),s(),o(null),c(null),j(!1),S(null),_(null),f(null),w(null),d(!1)},disabled:t,children:"Refresh"}),e.jsx("button",{className:"btn btn-secondary",onClick:U,disabled:k||t,children:k?"Rebuilding...":"Rebuild Index"}),i&&!h&&e.jsx("button",{className:"btn btn-danger",onClick:z,disabled:v,children:"Remove Model"}),i&&h&&e.jsxs(e.Fragment,{children:[e.jsx("button",{className:"btn btn-danger",onClick:Ie,disabled:v,children:v?"Removing...":"Yes, Remove"}),e.jsx("button",{className:"btn btn-secondary",onClick:I,disabled:v,children:"Cancel"})]})]}),g&&e.jsx("div",{className:"alert alert-error",style:{marginTop:"8px"},children:g}),p&&e.jsx("div",{className:"alert alert-success",style:{marginTop:"8px"},children:"Index rebuilt successfully"})]}),m&&e.jsx("div",{className:"alert alert-error",children:m}),u&&e.jsx("div",{className:"card",children:e.jsx("div",{className:"loading",children:"Loading model details"})}),a&&!u&&e.jsxs("div",{className:"card",children:[e.jsx("h3",{style:{marginBottom:"16px"},children:a.id}),e.jsxs("div",{className:"model-meta",children:[e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Owner"}),e.jsx("span",{children:a.owned_by})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Size"}),e.jsx("span",{children:Ia(a.size)})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Created"}),e.jsx("span",{children:new Date(a.created).toLocaleString()})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Has Projection"}),e.jsx("span",{className:`badge ${a.has_projection?"badge-yes":"badge-no"}`,children:a.has_projection?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Has Encoder"}),e.jsx("span",{className:`badge ${a.has_encoder?"badge-yes":"badge-no"}`,children:a.has_encoder?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Has Decoder"}),e.jsx("span",{className:`badge ${a.has_decoder?"badge-yes":"badge-no"}`,children:a.has_decoder?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Is Recurrent"}),e.jsx("span",{className:`badge ${a.is_recurrent?"badge-yes":"badge-no"}`,children:a.is_recurrent?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Is Hybrid"}),e.jsx("span",{className:`badge ${a.is_hybrid?"badge-yes":"badge-no"}`,children:a.is_hybrid?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Is GPT"}),e.jsx("span",{className:`badge ${a.is_gpt?"badge-yes":"badge-no"}`,children:a.is_gpt?"Yes":"No"})]})]}),a.desc&&e.jsxs("div",{style:{marginTop:"16px"},children:[e.jsx("label",{style:{fontWeight:500,display:"block",marginBottom:"8px"},children:"Description"}),e.jsx("p",{children:a.desc})]}),a.metadata&&Object.keys(a.metadata).filter(L=>L!=="tokenizer.chat_template").length>0&&e.jsxs("div",{style:{marginTop:"16px"},children:[e.jsx("label",{style:{fontWeight:500,display:"block",marginBottom:"8px"},children:"Metadata"}),e.jsx("div",{className:"model-meta",children:Object.entries(a.metadata).filter(([L])=>L!=="tokenizer.chat_template").map(([L,G])=>e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:L}),e.jsx("span",{children:G})]},L))})]})]})]})}function Jp(n){if(n===0)return"0 B";const t=1024,r=["B","KB","MB","GB","TB"],s=Math.floor(Math.log(n)/Math.log(t));return parseFloat((n/Math.pow(t,s)).toFixed(2))+" "+r[s]}function ef(n){return new Date(n).toLocaleString()}function nf(){const[n,t]=y.useState(null),[r,s]=y.useState(!0),[l,i]=y.useState(null);y.useEffect(()=>{o()},[]);const o=async()=>{s(!0),i(null);try{const a=await fe.listRunningModels();t(a)}catch(a){i(a instanceof Error?a.message:"Failed to load running models")}finally{s(!1)}};return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Running Models"}),e.jsx("p",{children:"Models currently loaded in cache"})]}),e.jsxs("div",{className:"card",children:[r&&e.jsx("div",{className:"loading",children:"Loading running models"}),l&&e.jsx("div",{className:"alert alert-error",children:l}),!r&&!l&&n&&e.jsx("div",{className:"table-container",children:n.length>0?e.jsxs("table",{children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"ID"}),e.jsx("th",{children:"Owner"}),e.jsx("th",{children:"Family"}),e.jsx("th",{children:"Size"}),e.jsx("th",{children:"Expires At"}),e.jsx("th",{children:"Active Streams"})]})}),e.jsx("tbody",{children:n.map(a=>e.jsxs("tr",{children:[e.jsx("td",{children:a.id}),e.jsx("td",{children:a.owned_by}),e.jsx("td",{children:a.model_family}),e.jsx("td",{children:Jp(a.size)}),e.jsx("td",{children:ef(a.expires_at)}),e.jsx("td",{children:a.active_streams})]},a.id))})]}):e.jsxs("div",{className:"empty-state",children:[e.jsx("h3",{children:"No running models"}),e.jsx("p",{children:"Models will appear here when loaded into cache"})]})}),e.jsx("div",{style:{marginTop:"16px"},children:e.jsx("button",{className:"btn btn-secondary",onClick:o,disabled:r,children:"Refresh"})})]})]})}function tf(){const{download:n,isDownloading:t,startDownload:r,cancelDownload:s,clearDownload:l}=cu(),[i,o]=y.useState(""),a=x=>{x.preventDefault(),!(!i.trim()||t)&&r(i.trim())},c=(n==null?void 0:n.status)==="complete",u=(n==null?void 0:n.status)==="error";return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Pull Model"}),e.jsx("p",{children:"Download a model from a URL"}),e.jsx("p",{children:"Example: https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"})]}),e.jsxs("div",{className:"card",children:[e.jsxs("form",{onSubmit:a,children:[e.jsxs("div",{className:"form-group",children:[e.jsx("label",{htmlFor:"modelUrl",children:"Model URL"}),e.jsx("input",{type:"text",id:"modelUrl",value:i,onChange:x=>o(x.target.value),placeholder:"https://huggingface.co/...",disabled:t})]}),e.jsxs("div",{style:{display:"flex",gap:"12px"},children:[e.jsx("button",{className:"btn btn-primary",type:"submit",disabled:t||!i.trim(),children:t?"Downloading...":"Pull Model"}),t&&e.jsx("button",{className:"btn btn-danger",type:"button",onClick:s,children:"Cancel"}),(c||u)&&e.jsx("button",{className:"btn",type:"button",onClick:l,children:"Clear"})]})]}),n&&n.messages.length>0&&e.jsx("div",{className:"status-box",children:n.messages.map((x,m)=>e.jsx("div",{className:`status-line ${x.type}`,children:x.text},m))})]})]})}function rf(){var U;const{invalidate:n}=Bs(),[t,r]=y.useState(null),[s,l]=y.useState(!0),[i,o]=y.useState(null),[a,c]=y.useState(null),[u,x]=y.useState(null),[m,f]=y.useState(!1),[k,N]=y.useState(null),[g,w]=y.useState("details"),[p,d]=y.useState(!1),[h,j]=y.useState([]),v=y.useRef(null);y.useEffect(()=>{b()},[]);const b=async()=>{l(!0),o(null);try{const A=await fe.listCatalog();r(A)}catch(A){o(A instanceof Error?A.message:"Failed to load catalog")}finally{l(!1)}},E=async A=>{if(a===A){c(null),x(null),w("details"),j([]);return}c(A),w("details"),j([]),f(!0),N(null),x(null);try{const z=await fe.showCatalogModel(A);x(z)}catch(z){N(z instanceof Error?z.message:"Failed to load model info")}finally{f(!1)}},S=()=>{if(!a)return;d(!0),j([]),w("pull");const A="\r\x1B[K",z=(I,L)=>{j(G=>[...G,{text:I,type:L}])},Ie=(I,L)=>{j(G=>{if(G.length===0)return[{text:I,type:L}];const R=[...G];return R[R.length-1]={text:I,type:L},R})};v.current=fe.pullCatalogModel(a,I=>{if(I.status)if(I.status.startsWith(A)){const L=I.status.slice(A.length);Ie(L,"info")}else z(I.status,"info");I.model_file&&z(`Model file: ${I.model_file}`,"info")},I=>{z(I,"error"),d(!1)},()=>{z("Pull complete!","success"),d(!1),n(),b()})},C=()=>{v.current&&(v.current(),v.current=null),d(!1),j(A=>[...A,{text:"Cancelled",type:"error"}])},_=((U=t==null?void 0:t.find(A=>A.id===a))==null?void 0:U.downloaded)??!1;return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Catalog"}),e.jsx("p",{children:"Browse available models in the catalog. Click a model to view details."})]}),e.jsxs("div",{className:"card",children:[s&&e.jsx("div",{className:"loading",children:"Loading catalog"}),i&&e.jsx("div",{className:"alert alert-error",children:i}),!s&&!i&&t&&e.jsx("div",{className:"table-container",children:t.length>0?e.jsxs("table",{children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{style:{width:"40px",textAlign:"center"},title:"Validated",children:"✓"}),e.jsx("th",{children:"ID"}),e.jsx("th",{children:"Category"}),e.jsx("th",{children:"Owner"}),e.jsx("th",{children:"Family"}),e.jsx("th",{children:"Downloaded"}),e.jsx("th",{children:"Capabilities"})]})}),e.jsx("tbody",{children:t.map(A=>e.jsxs("tr",{onClick:()=>E(A.id),className:a===A.id?"selected":"",style:{cursor:"pointer"},children:[e.jsx("td",{style:{textAlign:"center",color:A.validated?"inherit":"#e74c3c"},children:A.validated?"✓":"✗"}),e.jsx("td",{children:A.id}),e.jsx("td",{children:A.category}),e.jsx("td",{children:A.owned_by}),e.jsx("td",{children:A.model_family}),e.jsx("td",{children:e.jsx("span",{className:`badge ${A.downloaded?"badge-yes":"badge-no"}`,children:A.downloaded?"Yes":"No"})}),e.jsxs("td",{children:[A.capabilities.images&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Images"}),A.capabilities.audio&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Audio"}),A.capabilities.video&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Video"}),A.capabilities.streaming&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Streaming"}),A.capabilities.reasoning&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Reasoning"}),A.capabilities.tooling&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Tooling"}),A.capabilities.embedding&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Embedding"}),A.capabilities.rerank&&e.jsx("span",{className:"badge badge-yes",style:{marginRight:4},children:"Rerank"})]})]},A.id))})]}):e.jsxs("div",{className:"empty-state",children:[e.jsx("h3",{children:"No catalog entries"}),e.jsx("p",{children:"The catalog is empty"})]})}),e.jsxs("div",{style:{marginTop:"16px",display:"flex",gap:"8px"},children:[e.jsx("button",{className:"btn btn-secondary",onClick:()=>{b(),c(null),x(null),j([]),w("details"),N(null)},disabled:s,children:"Refresh"}),a&&e.jsx("button",{className:"btn btn-primary",onClick:S,disabled:p||_,children:p?"Pulling...":_?"Already Downloaded":"Pull Model"}),p&&e.jsx("button",{className:"btn btn-danger",onClick:C,children:"Cancel"})]})]}),k&&e.jsx("div",{className:"alert alert-error",children:k}),m&&e.jsx("div",{className:"card",children:e.jsx("div",{className:"loading",children:"Loading model details"})}),a&&!m&&(u||h.length>0)&&e.jsxs("div",{className:"card",children:[e.jsxs("div",{className:"tabs",children:[e.jsx("button",{className:`tab ${g==="details"?"active":""}`,onClick:()=>w("details"),children:"Details"}),e.jsx("button",{className:`tab ${g==="pull"?"active":""}`,onClick:()=>w("pull"),disabled:h.length===0&&!p,children:"Pull Output"})]}),g==="details"&&u&&e.jsxs(e.Fragment,{children:[e.jsx("h3",{style:{marginBottom:"16px"},children:u.id}),e.jsxs("div",{className:"model-meta",children:[e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Category"}),e.jsx("span",{children:u.category})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Owner"}),e.jsx("span",{children:u.owned_by})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Family"}),e.jsx("span",{children:u.model_family})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Downloaded"}),e.jsx("span",{className:`badge ${u.downloaded?"badge-yes":"badge-no"}`,children:u.downloaded?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Gated Model"}),e.jsx("span",{className:`badge ${u.gated_model?"badge-yes":"badge-no"}`,children:u.gated_model?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Endpoint"}),e.jsx("span",{children:u.capabilities.endpoint})]})]}),e.jsxs("div",{style:{marginTop:"24px"},children:[e.jsx("h4",{style:{marginBottom:"12px"},children:"Capabilities"}),e.jsxs("div",{className:"model-meta",children:[e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Images"}),e.jsx("span",{className:`badge ${u.capabilities.images?"badge-yes":"badge-no"}`,children:u.capabilities.images?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Audio"}),e.jsx("span",{className:`badge ${u.capabilities.audio?"badge-yes":"badge-no"}`,children:u.capabilities.audio?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Video"}),e.jsx("span",{className:`badge ${u.capabilities.video?"badge-yes":"badge-no"}`,children:u.capabilities.video?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Streaming"}),e.jsx("span",{className:`badge ${u.capabilities.streaming?"badge-yes":"badge-no"}`,children:u.capabilities.streaming?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Reasoning"}),e.jsx("span",{className:`badge ${u.capabilities.reasoning?"badge-yes":"badge-no"}`,children:u.capabilities.reasoning?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Tooling"}),e.jsx("span",{className:`badge ${u.capabilities.tooling?"badge-yes":"badge-no"}`,children:u.capabilities.tooling?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Embedding"}),e.jsx("span",{className:`badge ${u.capabilities.embedding?"badge-yes":"badge-no"}`,children:u.capabilities.embedding?"Yes":"No"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Rerank"}),e.jsx("span",{className:`badge ${u.capabilities.rerank?"badge-yes":"badge-no"}`,children:u.capabilities.rerank?"Yes":"No"})]})]})]}),e.jsxs("div",{style:{marginTop:"24px"},children:[e.jsx("h4",{style:{marginBottom:"12px"},children:"Web Page"}),e.jsx("div",{className:"model-meta-item",children:e.jsx("span",{children:u.web_page?e.jsx("a",{href:u.web_page,target:"_blank",rel:"noopener noreferrer",children:u.web_page}):"-"})})]}),e.jsxs("div",{style:{marginTop:"24px"},children:[e.jsx("h4",{style:{marginBottom:"12px"},children:"Files"}),e.jsxs("div",{className:"model-meta-item",style:{marginBottom:"12px"},children:[e.jsx("label",{children:"Model URL"}),e.jsx("span",{children:u.files.model.length>0?u.files.model.map((A,z)=>e.jsxs("div",{children:[A.url," ",A.size&&`(${A.size})`]},z)):"-"})]}),e.jsxs("div",{className:"model-meta-item",style:{marginBottom:"12px"},children:[e.jsx("label",{children:"Projection URL"}),e.jsx("span",{children:u.files.proj.length>0?u.files.proj.map((A,z)=>e.jsxs("div",{children:[A.url," ",A.size&&`(${A.size})`]},z)):"-"})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Template File"}),e.jsx("span",{children:u.template||"-"})]})]}),u.metadata.description&&e.jsxs("div",{style:{marginTop:"24px"},children:[e.jsx("h4",{style:{marginBottom:"12px"},children:"Description"}),e.jsx("div",{className:"model-meta-item",children:e.jsx("span",{children:u.metadata.description})})]}),e.jsxs("div",{style:{marginTop:"24px"},children:[e.jsx("h4",{style:{marginBottom:"12px"},children:"Metadata"}),e.jsxs("div",{className:"model-meta",children:[e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Created"}),e.jsx("span",{children:new Date(u.metadata.created).toLocaleString()})]}),e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Collections"}),e.jsx("span",{children:u.metadata.collections||"-"})]})]})]})]}),g==="pull"&&e.jsxs("div",{children:[e.jsxs("h3",{style:{marginBottom:"16px"},children:["Pull Output: ",a]}),h.length>0?e.jsx("div",{className:"status-box",children:h.map((A,z)=>e.jsx("div",{className:`status-line ${A.type}`,children:A.text},z))}):e.jsx("p",{children:"No pull output yet."}),p&&e.jsx("button",{className:"btn btn-danger",onClick:C,style:{marginTop:"16px"},children:"Cancel"})]})]})]})}function sf(){const[n,t]=y.useState(!1),[r,s]=y.useState([]),[l,i]=y.useState(null),[o,a]=y.useState(!0),c=y.useRef(null);y.useEffect(()=>{fe.getLibsVersion().then(i).catch(()=>{}).finally(()=>a(!1))},[]);const u=()=>{t(!0),s([]),i(null);const m=(f,k)=>{s(N=>[...N,{text:f,type:k}])};c.current=fe.pullLibs(f=>{f.status&&m(f.status,"info"),(f.current||f.latest)&&i(f)},f=>{m(f,"error"),t(!1)},()=>{m("Libs update complete!","success"),t(!1)})},x=()=>{c.current&&(c.current(),c.current=null),t(!1),s(m=>[...m,{text:"Cancelled",type:"error"}])};return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Pull/Update Libs"}),e.jsx("p",{children:"Download or update the Kronk libraries"})]}),e.jsxs("div",{className:"card",children:[o?e.jsx("p",{children:"Loading version info..."}):l?e.jsxs("div",{style:{marginBottom:"24px"},children:[e.jsx("h4",{style:{marginBottom:"12px"},children:"Current Version"}),e.jsxs("div",{className:"model-meta",children:[l.arch&&e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Architecture"}),e.jsx("span",{children:l.arch})]}),l.os&&e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"OS"}),e.jsx("span",{children:l.os})]}),l.processor&&e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Processor"}),e.jsx("span",{children:l.processor})]}),l.current&&e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Installed Version"}),e.jsx("span",{children:l.current})]}),l.latest&&e.jsxs("div",{className:"model-meta-item",children:[e.jsx("label",{children:"Latest Version"}),e.jsx("span",{children:l.latest})]})]})]}):e.jsx("p",{style:{marginBottom:"24px",color:"var(--color-gray-600)"},children:"No libs installed yet."}),e.jsxs("div",{style:{display:"flex",gap:"12px"},children:[e.jsx("button",{className:"btn btn-primary",onClick:u,disabled:n,children:n?"Updating...":"Pull/Update Libs"}),n&&e.jsx("button",{className:"btn btn-danger",onClick:x,children:"Cancel"})]}),r.length>0&&e.jsx("div",{className:"status-box",children:r.map((m,f)=>e.jsx("div",{className:`status-line ${m.type}`,children:m.text},f))})]})]})}const jl="kronk_token",du=y.createContext(null);function lf({children:n}){const[t,r]=y.useState(()=>localStorage.getItem(jl)||"");y.useEffect(()=>{t?localStorage.setItem(jl,t):localStorage.removeItem(jl)},[t]);const s=y.useCallback(i=>{r(i)},[]),l=y.useCallback(()=>{r("")},[]);return e.jsx(du.Provider,{value:{token:t,setToken:s,clearToken:l,hasToken:!!t},children:n})}function Nr(){const n=y.useContext(du);if(!n)throw new Error("useToken must be used within a TokenProvider");return n}function of(){const{token:n}=Nr(),[t,r]=y.useState(null),[s,l]=y.useState(!1),[i,o]=y.useState(null),a=async()=>{if(n){l(!0),o(null);try{const c=await fe.listKeys(n);r(c)}catch(c){o(c instanceof Error?c.message:"Failed to load keys"),r(null)}finally{l(!1)}}};return y.useEffect(()=>{n&&a()},[n]),e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Security Keys"}),e.jsx("p",{children:"List all security keys (requires admin token)"})]}),!n&&e.jsxs("div",{className:"alert alert-error",children:["No API token configured. ",e.jsx(Bn,{to:"/settings",children:"Configure your token in Settings"})]}),n&&e.jsx("div",{className:"card",children:e.jsx("button",{className:"btn btn-primary",onClick:a,disabled:s,children:s?"Loading...":"Refresh Keys"})}),i&&e.jsx("div",{className:"alert alert-error",children:i}),t&&e.jsx("div",{className:"card",children:e.jsx("div",{className:"table-container",children:t.length>0?e.jsxs("table",{children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"ID"}),e.jsx("th",{children:"Created"})]})}),e.jsx("tbody",{children:t.map(c=>e.jsxs("tr",{children:[e.jsx("td",{children:c.id}),e.jsx("td",{children:new Date(c.created).toLocaleString()})]},c.id))})]}):e.jsxs("div",{className:"empty-state",children:[e.jsx("h3",{children:"No keys found"}),e.jsx("p",{children:"Create a key to get started"})]})})})]})}function af(){const{token:n}=Nr(),[t,r]=y.useState(!1),[s,l]=y.useState(null),[i,o]=y.useState(null),a=async c=>{if(c.preventDefault(),!!n){r(!0),l(null),o(null);try{const u=await fe.createKey(n);o(u.id)}catch(u){l(u instanceof Error?u.message:"Failed to create key")}finally{r(!1)}}};return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Create Security Key"}),e.jsx("p",{children:"Generate a new security key (requires admin token)"})]}),!n&&e.jsxs("div",{className:"alert alert-error",children:["No API token configured. ",e.jsx(Bn,{to:"/settings",children:"Configure your token in Settings"})]}),n&&e.jsx("div",{className:"card",children:e.jsx("form",{onSubmit:a,children:e.jsx("button",{className:"btn btn-primary",type:"submit",disabled:t,children:t?"Creating...":"Create Key"})})}),s&&e.jsx("div",{className:"alert alert-error",children:s}),i&&e.jsxs("div",{className:"card",children:[e.jsx("div",{className:"alert alert-success",children:"Key created successfully!"}),e.jsxs("div",{style:{marginTop:"12px"},children:[e.jsx("label",{style:{fontWeight:500,display:"block",marginBottom:"8px"},children:"New Key ID"}),e.jsx("div",{className:"token-display",children:i}),e.jsx("p",{style:{marginTop:"8px",fontSize:"13px",color:"var(--color-gray-600)"},children:"Store this key securely. It will not be shown again."})]})]})]})}function cf(){const{token:n}=Nr(),[t,r]=y.useState(""),[s,l]=y.useState(!1),[i,o]=y.useState(null),[a,c]=y.useState(null),u=async x=>{if(x.preventDefault(),!(!n||!t.trim())){l(!0),o(null),c(null);try{await fe.deleteKey(n,t.trim()),c(`Key "${t}" deleted successfully`),r("")}catch(m){o(m instanceof Error?m.message:"Failed to delete key")}finally{l(!1)}}};return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Delete Security Key"}),e.jsx("p",{children:"Remove a security key (requires admin token)"})]}),!n&&e.jsxs("div",{className:"alert alert-error",children:["No API token configured. ",e.jsx(Bn,{to:"/settings",children:"Configure your token in Settings"})]}),n&&e.jsxs("div",{className:"card",children:[i&&e.jsx("div",{className:"alert alert-error",children:i}),a&&e.jsx("div",{className:"alert alert-success",children:a}),e.jsxs("form",{onSubmit:u,children:[e.jsxs("div",{className:"form-group",children:[e.jsx("label",{htmlFor:"keyId",children:"Key ID"}),e.jsx("input",{type:"text",id:"keyId",value:t,onChange:x=>r(x.target.value),placeholder:"Enter key ID to delete"})]}),e.jsx("button",{className:"btn btn-danger",type:"submit",disabled:s||!t.trim(),children:s?"Deleting...":"Delete Key"})]})]})]})}const Oa=[{label:"/v1/chat/completions",value:"chat-completions"},{label:"/v1/embeddings",value:"embeddings"}],df=[{label:"Unlimited",value:"unlimited"},{label:"Per Day",value:"day"},{label:"Per Month",value:"month"},{label:"Per Year",value:"year"}],uf=()=>({enabled:!1,limit:1e3,window:"unlimited"});function hf(){const{token:n}=Nr(),[t,r]=y.useState(!1),[s,l]=y.useState(()=>{const p={};return Oa.forEach(d=>{p[d.value]=uf()}),p}),[i,o]=y.useState("24"),[a,c]=y.useState("h"),[u,x]=y.useState(!1),[m,f]=y.useState(null),[k,N]=y.useState(null),g=(p,d)=>{l(h=>({...h,[p]:{...h[p],...d}}))},w=async p=>{if(p.preventDefault(),!n)return;x(!0),f(null),N(null);const d=parseInt(i);let h;switch(a){case"h":h=d*60*60*1e9;break;case"d":h=d*24*60*60*1e9;break;case"M":h=d*30*24*60*60*1e9;break;case"y":h=d*365*24*60*60*1e9;break}const j={};Object.entries(s).forEach(([v,b])=>{b.enabled&&(j[v]={limit:b.window==="unlimited"?0:b.limit,window:b.window})});try{const v=await fe.createToken(n,{admin:t,endpoints:j,duration:h});N(v.token)}catch(v){f(v instanceof Error?v.message:"Failed to create token")}finally{x(!1)}};return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Create Token"}),e.jsx("p",{children:"Generate a new authentication token"})]}),!n&&e.jsxs("div",{className:"alert alert-error",children:["No API token configured. ",e.jsx(Bn,{to:"/settings",children:"Configure your token in Settings"})]}),n&&e.jsx("div",{className:"card",children:e.jsxs("form",{onSubmit:w,children:[e.jsx("div",{className:"form-group",children:e.jsxs("label",{children:[e.jsx("input",{type:"checkbox",checked:t,onChange:p=>r(p.target.checked)}),"Admin privileges"]})}),e.jsxs("div",{className:"form-group",children:[e.jsx("label",{children:"Endpoints & Rate Limits"}),e.jsx("div",{style:{display:"flex",flexDirection:"column",gap:"12px"},children:Oa.map(p=>{const d=s[p.value];return e.jsxs("div",{style:{padding:"12px",background:d.enabled?"rgba(240, 181, 49, 0.1)":"var(--color-gray-100)",borderRadius:"6px",border:d.enabled?"1px solid rgba(240, 181, 49, 0.3)":"1px solid transparent"},children:[e.jsxs("label",{style:{display:"flex",alignItems:"center",cursor:"pointer",fontWeight:500},children:[e.jsx("input",{type:"checkbox",checked:d.enabled,onChange:h=>g(p.value,{enabled:h.target.checked}),style:{marginRight:"8px"}}),p.label]}),d.enabled&&e.jsxs("div",{style:{display:"flex",gap:"12px",marginTop:"10px",paddingLeft:"24px"},children:[e.jsxs("div",{style:{flex:1},children:[e.jsx("label",{style:{fontSize:"12px",color:"var(--color-gray-600)",display:"block",marginBottom:"4px"},children:"Rate Limit"}),e.jsx("select",{value:d.window,onChange:h=>g(p.value,{window:h.target.value}),style:{width:"100%"},children:df.map(h=>e.jsx("option",{value:h.value,children:h.label},h.value))})]}),d.window!=="unlimited"&&e.jsxs("div",{style:{flex:1},children:[e.jsx("label",{style:{fontSize:"12px",color:"var(--color-gray-600)",display:"block",marginBottom:"4px"},children:"Max Requests"}),e.jsx("input",{type:"number",value:d.limit,onChange:h=>g(p.value,{limit:parseInt(h.target.value)||0}),min:"1",style:{width:"100%"}})]})]})]},p.value)})})]}),e.jsxs("div",{className:"form-row",children:[e.jsxs("div",{className:"form-group",children:[e.jsx("label",{htmlFor:"duration",children:"Duration"}),e.jsx("input",{type:"number",id:"duration",value:i,onChange:p=>o(p.target.value),min:"1"})]}),e.jsxs("div",{className:"form-group",children:[e.jsx("label",{htmlFor:"durationUnit",children:"Unit"}),e.jsxs("select",{id:"durationUnit",value:a,onChange:p=>c(p.target.value),children:[e.jsx("option",{value:"h",children:"Hours"}),e.jsx("option",{value:"d",children:"Days"}),e.jsx("option",{value:"M",children:"Months"}),e.jsx("option",{value:"y",children:"Years"})]})]})]}),e.jsx("button",{className:"btn btn-primary",type:"submit",disabled:u,children:u?"Creating...":"Create Token"})]})}),m&&e.jsx("div",{className:"alert alert-error",children:m}),k&&e.jsxs("div",{className:"card",children:[e.jsx("div",{className:"alert alert-success",children:"Token created successfully!"}),e.jsxs("div",{style:{marginTop:"12px"},children:[e.jsx("label",{style:{fontWeight:500,display:"block",marginBottom:"8px"},children:"Token"}),e.jsx("div",{className:"token-display",children:k}),e.jsx("p",{style:{marginTop:"8px",fontSize:"13px",color:"var(--color-gray-600)"},children:"Store this token securely. It will not be shown again."})]})]})]})}function mf(){const{token:n,setToken:t,clearToken:r,hasToken:s}=Nr(),[l,i]=y.useState(n),[o,a]=y.useState(!1),[c,u]=y.useState(!1),x=f=>{f.preventDefault(),t(l.trim()),u(!0),setTimeout(()=>u(!1),2e3)},m=()=>{r(),i(""),u(!0),setTimeout(()=>u(!1),2e3)};return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Settings"}),e.jsx("p",{children:"Configure your API token for authenticated requests"})]}),e.jsx("div",{className:"card",children:e.jsxs("form",{onSubmit:x,children:[e.jsxs("div",{className:"form-group",children:[e.jsx("label",{htmlFor:"apiToken",children:"API Token"}),e.jsxs("div",{style:{display:"flex",gap:"8px"},children:[e.jsx("input",{type:o?"text":"password",id:"apiToken",value:l,onChange:f=>i(f.target.value),placeholder:"Enter your KRONK_TOKEN",style:{flex:1}}),e.jsx("button",{type:"button",className:"btn btn-secondary",onClick:()=>a(!o),children:o?"Hide":"Show"})]}),e.jsx("p",{style:{fontSize:"12px",color:"var(--color-gray-600)",marginTop:"8px"},children:"This token will be stored in your browser and used for all API requests that require authentication."})]}),e.jsxs("div",{style:{display:"flex",gap:"12px"},children:[e.jsx("button",{className:"btn btn-primary",type:"submit",children:"Save Token"}),s&&e.jsx("button",{className:"btn btn-danger",type:"button",onClick:m,children:"Clear Token"})]})]})}),c&&e.jsx("div",{className:"alert alert-success",children:"Token settings saved"}),e.jsxs("div",{className:"card",children:[e.jsx("h4",{style:{marginBottom:"12px",color:"var(--color-blue)"},children:"Token Status"}),e.jsx("p",{style:{color:s?"var(--color-success)":"var(--color-gray-600)"},children:s?"✓ Token is configured":"○ No token configured"})]})]})}var uu={exports:{}};(function(n){var t=typeof window<"u"?window:typeof WorkerGlobalScope<"u"&&self instanceof WorkerGlobalScope?self:{};/**
 * Prism: Lightweight, robust, elegant syntax highlighting
 *
 * @license MIT <https://opensource.org/licenses/MIT>
 * @author Lea Verou <https://lea.verou.me>
 * @namespace
 * @public
 */var r=function(s){var l=/(?:^|\s)lang(?:uage)?-([\w-]+)(?=\s|$)/i,i=0,o={},a={manual:s.Prism&&s.Prism.manual,disableWorkerMessageHandler:s.Prism&&s.Prism.disableWorkerMessageHandler,util:{encode:function d(h){return h instanceof c?new c(h.type,d(h.content),h.alias):Array.isArray(h)?h.map(d):h.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/\u00a0/g," ")},type:function(d){return Object.prototype.toString.call(d).slice(8,-1)},objId:function(d){return d.__id||Object.defineProperty(d,"__id",{value:++i}),d.__id},clone:function d(h,j){j=j||{};var v,b;switch(a.util.type(h)){case"Object":if(b=a.util.objId(h),j[b])return j[b];v={},j[b]=v;for(var E in h)h.hasOwnProperty(E)&&(v[E]=d(h[E],j));return v;case"Array":return b=a.util.objId(h),j[b]?j[b]:(v=[],j[b]=v,h.forEach(function(S,C){v[C]=d(S,j)}),v);default:return h}},getLanguage:function(d){for(;d;){var h=l.exec(d.className);if(h)return h[1].toLowerCase();d=d.parentElement}return"none"},setLanguage:function(d,h){d.className=d.className.replace(RegExp(l,"gi"),""),d.classList.add("language-"+h)},currentScript:function(){if(typeof document>"u")return null;if(document.currentScript&&document.currentScript.tagName==="SCRIPT")return document.currentScript;try{throw new Error}catch(v){var d=(/at [^(\r\n]*\((.*):[^:]+:[^:]+\)$/i.exec(v.stack)||[])[1];if(d){var h=document.getElementsByTagName("script");for(var j in h)if(h[j].src==d)return h[j]}return null}},isActive:function(d,h,j){for(var v="no-"+h;d;){var b=d.classList;if(b.contains(h))return!0;if(b.contains(v))return!1;d=d.parentElement}return!!j}},languages:{plain:o,plaintext:o,text:o,txt:o,extend:function(d,h){var j=a.util.clone(a.languages[d]);for(var v in h)j[v]=h[v];return j},insertBefore:function(d,h,j,v){v=v||a.languages;var b=v[d],E={};for(var S in b)if(b.hasOwnProperty(S)){if(S==h)for(var C in j)j.hasOwnProperty(C)&&(E[C]=j[C]);j.hasOwnProperty(S)||(E[S]=b[S])}var _=v[d];return v[d]=E,a.languages.DFS(a.languages,function(U,A){A===_&&U!=d&&(this[U]=E)}),E},DFS:function d(h,j,v,b){b=b||{};var E=a.util.objId;for(var S in h)if(h.hasOwnProperty(S)){j.call(h,S,h[S],v||S);var C=h[S],_=a.util.type(C);_==="Object"&&!b[E(C)]?(b[E(C)]=!0,d(C,j,null,b)):_==="Array"&&!b[E(C)]&&(b[E(C)]=!0,d(C,j,S,b))}}},plugins:{},highlightAll:function(d,h){a.highlightAllUnder(document,d,h)},highlightAllUnder:function(d,h,j){var v={callback:j,container:d,selector:'code[class*="language-"], [class*="language-"] code, code[class*="lang-"], [class*="lang-"] code'};a.hooks.run("before-highlightall",v),v.elements=Array.prototype.slice.apply(v.container.querySelectorAll(v.selector)),a.hooks.run("before-all-elements-highlight",v);for(var b=0,E;E=v.elements[b++];)a.highlightElement(E,h===!0,v.callback)},highlightElement:function(d,h,j){var v=a.util.getLanguage(d),b=a.languages[v];a.util.setLanguage(d,v);var E=d.parentElement;E&&E.nodeName.toLowerCase()==="pre"&&a.util.setLanguage(E,v);var S=d.textContent,C={element:d,language:v,grammar:b,code:S};function _(A){C.highlightedCode=A,a.hooks.run("before-insert",C),C.element.innerHTML=C.highlightedCode,a.hooks.run("after-highlight",C),a.hooks.run("complete",C),j&&j.call(C.element)}if(a.hooks.run("before-sanity-check",C),E=C.element.parentElement,E&&E.nodeName.toLowerCase()==="pre"&&!E.hasAttribute("tabindex")&&E.setAttribute("tabindex","0"),!C.code){a.hooks.run("complete",C),j&&j.call(C.element);return}if(a.hooks.run("before-highlight",C),!C.grammar){_(a.util.encode(C.code));return}if(h&&s.Worker){var U=new Worker(a.filename);U.onmessage=function(A){_(A.data)},U.postMessage(JSON.stringify({language:C.language,code:C.code,immediateClose:!0}))}else _(a.highlight(C.code,C.grammar,C.language))},highlight:function(d,h,j){var v={code:d,grammar:h,language:j};if(a.hooks.run("before-tokenize",v),!v.grammar)throw new Error('The language "'+v.language+'" has no grammar.');return v.tokens=a.tokenize(v.code,v.grammar),a.hooks.run("after-tokenize",v),c.stringify(a.util.encode(v.tokens),v.language)},tokenize:function(d,h){var j=h.rest;if(j){for(var v in j)h[v]=j[v];delete h.rest}var b=new m;return f(b,b.head,d),x(d,b,h,b.head,0),N(b)},hooks:{all:{},add:function(d,h){var j=a.hooks.all;j[d]=j[d]||[],j[d].push(h)},run:function(d,h){var j=a.hooks.all[d];if(!(!j||!j.length))for(var v=0,b;b=j[v++];)b(h)}},Token:c};s.Prism=a;function c(d,h,j,v){this.type=d,this.content=h,this.alias=j,this.length=(v||"").length|0}c.stringify=function d(h,j){if(typeof h=="string")return h;if(Array.isArray(h)){var v="";return h.forEach(function(_){v+=d(_,j)}),v}var b={type:h.type,content:d(h.content,j),tag:"span",classes:["token",h.type],attributes:{},language:j},E=h.alias;E&&(Array.isArray(E)?Array.prototype.push.apply(b.classes,E):b.classes.push(E)),a.hooks.run("wrap",b);var S="";for(var C in b.attributes)S+=" "+C+'="'+(b.attributes[C]||"").replace(/"/g,"&quot;")+'"';return"<"+b.tag+' class="'+b.classes.join(" ")+'"'+S+">"+b.content+"</"+b.tag+">"};function u(d,h,j,v){d.lastIndex=h;var b=d.exec(j);if(b&&v&&b[1]){var E=b[1].length;b.index+=E,b[0]=b[0].slice(E)}return b}function x(d,h,j,v,b,E){for(var S in j)if(!(!j.hasOwnProperty(S)||!j[S])){var C=j[S];C=Array.isArray(C)?C:[C];for(var _=0;_<C.length;++_){if(E&&E.cause==S+","+_)return;var U=C[_],A=U.inside,z=!!U.lookbehind,Ie=!!U.greedy,I=U.alias;if(Ie&&!U.pattern.global){var L=U.pattern.toString().match(/[imsuy]*$/)[0];U.pattern=RegExp(U.pattern.source,L+"g")}for(var G=U.pattern||U,R=v.next,O=b;R!==h.tail&&!(E&&O>=E.reach);O+=R.value.length,R=R.next){var M=R.value;if(h.length>d.length)return;if(!(M instanceof c)){var B=1,F;if(Ie){if(F=u(G,O,d,z),!F||F.index>=d.length)break;var je=F.index,ne=F.index+F[0].length,se=O;for(se+=R.value.length;je>=se;)R=R.next,se+=R.value.length;if(se-=R.value.length,O=se,R.value instanceof c)continue;for(var Oe=R;Oe!==h.tail&&(se<ne||typeof Oe.value=="string");Oe=Oe.next)B++,se+=Oe.value.length;B--,M=d.slice(O,se),F.index-=O}else if(F=u(G,0,M,z),!F)continue;var je=F.index,Me=F[0],Rt=M.slice(0,je),Xn=M.slice(je+Me.length),zs=O+M.length;E&&zs>E.reach&&(E.reach=zs);var br=R.prev;Rt&&(br=f(h,br,Rt),O+=Rt.length),k(h,br,B);var hu=new c(S,A?a.tokenize(Me,A):Me,I,Me);if(R=f(h,br,hu),Xn&&f(h,R,Xn),B>1){var $s={cause:S+","+_,reach:zs};x(d,h,j,R.prev,O,$s),E&&$s.reach>E.reach&&(E.reach=$s.reach)}}}}}}function m(){var d={value:null,prev:null,next:null},h={value:null,prev:d,next:null};d.next=h,this.head=d,this.tail=h,this.length=0}function f(d,h,j){var v=h.next,b={value:j,prev:h,next:v};return h.next=b,v.prev=b,d.length++,b}function k(d,h,j){for(var v=h.next,b=0;b<j&&v!==d.tail;b++)v=v.next;h.next=v,v.prev=h,d.length-=b}function N(d){for(var h=[],j=d.head.next;j!==d.tail;)h.push(j.value),j=j.next;return h}if(!s.document)return s.addEventListener&&(a.disableWorkerMessageHandler||s.addEventListener("message",function(d){var h=JSON.parse(d.data),j=h.language,v=h.code,b=h.immediateClose;s.postMessage(a.highlight(v,a.languages[j],j)),b&&s.close()},!1)),a;var g=a.util.currentScript();g&&(a.filename=g.src,g.hasAttribute("data-manual")&&(a.manual=!0));function w(){a.manual||a.highlightAll()}if(!a.manual){var p=document.readyState;p==="loading"||p==="interactive"&&g&&g.defer?document.addEventListener("DOMContentLoaded",w):window.requestAnimationFrame?window.requestAnimationFrame(w):window.setTimeout(w,16)}return a}(t);n.exports&&(n.exports=r),typeof po<"u"&&(po.Prism=r),r.languages.markup={comment:{pattern:/<!--(?:(?!<!--)[\s\S])*?-->/,greedy:!0},prolog:{pattern:/<\?[\s\S]+?\?>/,greedy:!0},doctype:{pattern:/<!DOCTYPE(?:[^>"'[\]]|"[^"]*"|'[^']*')+(?:\[(?:[^<"'\]]|"[^"]*"|'[^']*'|<(?!!--)|<!--(?:[^-]|-(?!->))*-->)*\]\s*)?>/i,greedy:!0,inside:{"internal-subset":{pattern:/(^[^\[]*\[)[\s\S]+(?=\]>$)/,lookbehind:!0,greedy:!0,inside:null},string:{pattern:/"[^"]*"|'[^']*'/,greedy:!0},punctuation:/^<!|>$|[[\]]/,"doctype-tag":/^DOCTYPE/i,name:/[^\s<>'"]+/}},cdata:{pattern:/<!\[CDATA\[[\s\S]*?\]\]>/i,greedy:!0},tag:{pattern:/<\/?(?!\d)[^\s>\/=$<%]+(?:\s(?:\s*[^\s>\/=]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+(?=[\s>]))|(?=[\s/>])))+)?\s*\/?>/,greedy:!0,inside:{tag:{pattern:/^<\/?[^\s>\/]+/,inside:{punctuation:/^<\/?/,namespace:/^[^\s>\/:]+:/}},"special-attr":[],"attr-value":{pattern:/=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+)/,inside:{punctuation:[{pattern:/^=/,alias:"attr-equals"},{pattern:/^(\s*)["']|["']$/,lookbehind:!0}]}},punctuation:/\/?>/,"attr-name":{pattern:/[^\s>\/]+/,inside:{namespace:/^[^\s>\/:]+:/}}}},entity:[{pattern:/&[\da-z]{1,8};/i,alias:"named-entity"},/&#x?[\da-f]{1,8};/i]},r.languages.markup.tag.inside["attr-value"].inside.entity=r.languages.markup.entity,r.languages.markup.doctype.inside["internal-subset"].inside=r.languages.markup,r.hooks.add("wrap",function(s){s.type==="entity"&&(s.attributes.title=s.content.replace(/&amp;/,"&"))}),Object.defineProperty(r.languages.markup.tag,"addInlined",{value:function(l,i){var o={};o["language-"+i]={pattern:/(^<!\[CDATA\[)[\s\S]+?(?=\]\]>$)/i,lookbehind:!0,inside:r.languages[i]},o.cdata=/^<!\[CDATA\[|\]\]>$/i;var a={"included-cdata":{pattern:/<!\[CDATA\[[\s\S]*?\]\]>/i,inside:o}};a["language-"+i]={pattern:/[\s\S]+/,inside:r.languages[i]};var c={};c[l]={pattern:RegExp(/(<__[^>]*>)(?:<!\[CDATA\[(?:[^\]]|\](?!\]>))*\]\]>|(?!<!\[CDATA\[)[\s\S])*?(?=<\/__>)/.source.replace(/__/g,function(){return l}),"i"),lookbehind:!0,greedy:!0,inside:a},r.languages.insertBefore("markup","cdata",c)}}),Object.defineProperty(r.languages.markup.tag,"addAttribute",{value:function(s,l){r.languages.markup.tag.inside["special-attr"].push({pattern:RegExp(/(^|["'\s])/.source+"(?:"+s+")"+/\s*=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+(?=[\s>]))/.source,"i"),lookbehind:!0,inside:{"attr-name":/^[^\s=]+/,"attr-value":{pattern:/=[\s\S]+/,inside:{value:{pattern:/(^=\s*(["']|(?!["'])))\S[\s\S]*(?=\2$)/,lookbehind:!0,alias:[l,"language-"+l],inside:r.languages[l]},punctuation:[{pattern:/^=/,alias:"attr-equals"},/"|'/]}}}})}}),r.languages.html=r.languages.markup,r.languages.mathml=r.languages.markup,r.languages.svg=r.languages.markup,r.languages.xml=r.languages.extend("markup",{}),r.languages.ssml=r.languages.xml,r.languages.atom=r.languages.xml,r.languages.rss=r.languages.xml,function(s){var l=/(?:"(?:\\(?:\r\n|[\s\S])|[^"\\\r\n])*"|'(?:\\(?:\r\n|[\s\S])|[^'\\\r\n])*')/;s.languages.css={comment:/\/\*[\s\S]*?\*\//,atrule:{pattern:RegExp("@[\\w-](?:"+/[^;{\s"']|\s+(?!\s)/.source+"|"+l.source+")*?"+/(?:;|(?=\s*\{))/.source),inside:{rule:/^@[\w-]+/,"selector-function-argument":{pattern:/(\bselector\s*\(\s*(?![\s)]))(?:[^()\s]|\s+(?![\s)])|\((?:[^()]|\([^()]*\))*\))+(?=\s*\))/,lookbehind:!0,alias:"selector"},keyword:{pattern:/(^|[^\w-])(?:and|not|only|or)(?![\w-])/,lookbehind:!0}}},url:{pattern:RegExp("\\burl\\((?:"+l.source+"|"+/(?:[^\\\r\n()"']|\\[\s\S])*/.source+")\\)","i"),greedy:!0,inside:{function:/^url/i,punctuation:/^\(|\)$/,string:{pattern:RegExp("^"+l.source+"$"),alias:"url"}}},selector:{pattern:RegExp(`(^|[{}\\s])[^{}\\s](?:[^{};"'\\s]|\\s+(?![\\s{])|`+l.source+")*(?=\\s*\\{)"),lookbehind:!0},string:{pattern:l,greedy:!0},property:{pattern:/(^|[^-\w\xA0-\uFFFF])(?!\s)[-_a-z\xA0-\uFFFF](?:(?!\s)[-\w\xA0-\uFFFF])*(?=\s*:)/i,lookbehind:!0},important:/!important\b/i,function:{pattern:/(^|[^-a-z0-9])[-a-z0-9]+(?=\()/i,lookbehind:!0},punctuation:/[(){};:,]/},s.languages.css.atrule.inside.rest=s.languages.css;var i=s.languages.markup;i&&(i.tag.addInlined("style","css"),i.tag.addAttribute("style","css"))}(r),r.languages.clike={comment:[{pattern:/(^|[^\\])\/\*[\s\S]*?(?:\*\/|$)/,lookbehind:!0,greedy:!0},{pattern:/(^|[^\\:])\/\/.*/,lookbehind:!0,greedy:!0}],string:{pattern:/(["'])(?:\\(?:\r\n|[\s\S])|(?!\1)[^\\\r\n])*\1/,greedy:!0},"class-name":{pattern:/(\b(?:class|extends|implements|instanceof|interface|new|trait)\s+|\bcatch\s+\()[\w.\\]+/i,lookbehind:!0,inside:{punctuation:/[.\\]/}},keyword:/\b(?:break|catch|continue|do|else|finally|for|function|if|in|instanceof|new|null|return|throw|try|while)\b/,boolean:/\b(?:false|true)\b/,function:/\b\w+(?=\()/,number:/\b0x[\da-f]+\b|(?:\b\d+(?:\.\d*)?|\B\.\d+)(?:e[+-]?\d+)?/i,operator:/[<>]=?|[!=]=?=?|--?|\+\+?|&&?|\|\|?|[?*/~^%]/,punctuation:/[{}[\];(),.:]/},r.languages.javascript=r.languages.extend("clike",{"class-name":[r.languages.clike["class-name"],{pattern:/(^|[^$\w\xA0-\uFFFF])(?!\s)[_$A-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\.(?:constructor|prototype))/,lookbehind:!0}],keyword:[{pattern:/((?:^|\})\s*)catch\b/,lookbehind:!0},{pattern:/(^|[^.]|\.\.\.\s*)\b(?:as|assert(?=\s*\{)|async(?=\s*(?:function\b|\(|[$\w\xA0-\uFFFF]|$))|await|break|case|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally(?=\s*(?:\{|$))|for|from(?=\s*(?:['"]|$))|function|(?:get|set)(?=\s*(?:[#\[$\w\xA0-\uFFFF]|$))|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)\b/,lookbehind:!0}],function:/#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*(?:\.\s*(?:apply|bind|call)\s*)?\()/,number:{pattern:RegExp(/(^|[^\w$])/.source+"(?:"+(/NaN|Infinity/.source+"|"+/0[bB][01]+(?:_[01]+)*n?/.source+"|"+/0[oO][0-7]+(?:_[0-7]+)*n?/.source+"|"+/0[xX][\dA-Fa-f]+(?:_[\dA-Fa-f]+)*n?/.source+"|"+/\d+(?:_\d+)*n/.source+"|"+/(?:\d+(?:_\d+)*(?:\.(?:\d+(?:_\d+)*)?)?|\.\d+(?:_\d+)*)(?:[Ee][+-]?\d+(?:_\d+)*)?/.source)+")"+/(?![\w$])/.source),lookbehind:!0},operator:/--|\+\+|\*\*=?|=>|&&=?|\|\|=?|[!=]==|<<=?|>>>?=?|[-+*/%&|^!=<>]=?|\.{3}|\?\?=?|\?\.?|[~:]/}),r.languages.javascript["class-name"][0].pattern=/(\b(?:class|extends|implements|instanceof|interface|new)\s+)[\w.\\]+/,r.languages.insertBefore("javascript","keyword",{regex:{pattern:RegExp(/((?:^|[^$\w\xA0-\uFFFF."'\])\s]|\b(?:return|yield))\s*)/.source+/\//.source+"(?:"+/(?:\[(?:[^\]\\\r\n]|\\.)*\]|\\.|[^/\\\[\r\n])+\/[dgimyus]{0,7}/.source+"|"+/(?:\[(?:[^[\]\\\r\n]|\\.|\[(?:[^[\]\\\r\n]|\\.|\[(?:[^[\]\\\r\n]|\\.)*\])*\])*\]|\\.|[^/\\\[\r\n])+\/[dgimyus]{0,7}v[dgimyus]{0,7}/.source+")"+/(?=(?:\s|\/\*(?:[^*]|\*(?!\/))*\*\/)*(?:$|[\r\n,.;:})\]]|\/\/))/.source),lookbehind:!0,greedy:!0,inside:{"regex-source":{pattern:/^(\/)[\s\S]+(?=\/[a-z]*$)/,lookbehind:!0,alias:"language-regex",inside:r.languages.regex},"regex-delimiter":/^\/|\/$/,"regex-flags":/^[a-z]+$/}},"function-variable":{pattern:/#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*[=:]\s*(?:async\s*)?(?:\bfunction\b|(?:\((?:[^()]|\([^()]*\))*\)|(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)\s*=>))/,alias:"function"},parameter:[{pattern:/(function(?:\s+(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)?\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\))/,lookbehind:!0,inside:r.languages.javascript},{pattern:/(^|[^$\w\xA0-\uFFFF])(?!\s)[_$a-z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*=>)/i,lookbehind:!0,inside:r.languages.javascript},{pattern:/(\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*=>)/,lookbehind:!0,inside:r.languages.javascript},{pattern:/((?:\b|\s|^)(?!(?:as|async|await|break|case|catch|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally|for|from|function|get|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|set|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)(?![$\w\xA0-\uFFFF]))(?:(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*\s*)\(\s*|\]\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*\{)/,lookbehind:!0,inside:r.languages.javascript}],constant:/\b[A-Z](?:[A-Z_]|\dx?)*\b/}),r.languages.insertBefore("javascript","string",{hashbang:{pattern:/^#!.*/,greedy:!0,alias:"comment"},"template-string":{pattern:/`(?:\\[\s\S]|\$\{(?:[^{}]|\{(?:[^{}]|\{[^}]*\})*\})+\}|(?!\$\{)[^\\`])*`/,greedy:!0,inside:{"template-punctuation":{pattern:/^`|`$/,alias:"string"},interpolation:{pattern:/((?:^|[^\\])(?:\\{2})*)\$\{(?:[^{}]|\{(?:[^{}]|\{[^}]*\})*\})+\}/,lookbehind:!0,inside:{"interpolation-punctuation":{pattern:/^\$\{|\}$/,alias:"punctuation"},rest:r.languages.javascript}},string:/[\s\S]+/}},"string-property":{pattern:/((?:^|[,{])[ \t]*)(["'])(?:\\(?:\r\n|[\s\S])|(?!\2)[^\\\r\n])*\2(?=\s*:)/m,lookbehind:!0,greedy:!0,alias:"property"}}),r.languages.insertBefore("javascript","operator",{"literal-property":{pattern:/((?:^|[,{])[ \t]*)(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*:)/m,lookbehind:!0,alias:"property"}}),r.languages.markup&&(r.languages.markup.tag.addInlined("script","javascript"),r.languages.markup.tag.addAttribute(/on(?:abort|blur|change|click|composition(?:end|start|update)|dblclick|error|focus(?:in|out)?|key(?:down|up)|load|mouse(?:down|enter|leave|move|out|over|up)|reset|resize|scroll|select|slotchange|submit|unload|wheel)/.source,"javascript")),r.languages.js=r.languages.javascript,function(){if(typeof r>"u"||typeof document>"u")return;Element.prototype.matches||(Element.prototype.matches=Element.prototype.msMatchesSelector||Element.prototype.webkitMatchesSelector);var s="Loading…",l=function(g,w){return"✖ Error "+g+" while fetching file: "+w},i="✖ Error: File does not exist or is empty",o={js:"javascript",py:"python",rb:"ruby",ps1:"powershell",psm1:"powershell",sh:"bash",bat:"batch",h:"c",tex:"latex"},a="data-src-status",c="loading",u="loaded",x="failed",m="pre[data-src]:not(["+a+'="'+u+'"]):not(['+a+'="'+c+'"])';function f(g,w,p){var d=new XMLHttpRequest;d.open("GET",g,!0),d.onreadystatechange=function(){d.readyState==4&&(d.status<400&&d.responseText?w(d.responseText):d.status>=400?p(l(d.status,d.statusText)):p(i))},d.send(null)}function k(g){var w=/^\s*(\d+)\s*(?:(,)\s*(?:(\d+)\s*)?)?$/.exec(g||"");if(w){var p=Number(w[1]),d=w[2],h=w[3];return d?h?[p,Number(h)]:[p,void 0]:[p,p]}}r.hooks.add("before-highlightall",function(g){g.selector+=", "+m}),r.hooks.add("before-sanity-check",function(g){var w=g.element;if(w.matches(m)){g.code="",w.setAttribute(a,c);var p=w.appendChild(document.createElement("CODE"));p.textContent=s;var d=w.getAttribute("data-src"),h=g.language;if(h==="none"){var j=(/\.(\w+)$/.exec(d)||[,"none"])[1];h=o[j]||j}r.util.setLanguage(p,h),r.util.setLanguage(w,h);var v=r.plugins.autoloader;v&&v.loadLanguages(h),f(d,function(b){w.setAttribute(a,u);var E=k(w.getAttribute("data-range"));if(E){var S=b.split(/\r\n?|\n/g),C=E[0],_=E[1]==null?S.length:E[1];C<0&&(C+=S.length),C=Math.max(0,Math.min(C-1,S.length)),_<0&&(_+=S.length),_=Math.max(0,Math.min(_,S.length)),b=S.slice(C,_).join(`
`),w.hasAttribute("data-start")||w.setAttribute("data-start",String(C+1))}p.textContent=b,r.highlightElement(p)},function(b){w.setAttribute(a,x),p.textContent=b})}}),r.plugins.fileHighlight={highlight:function(w){for(var p=(w||document).querySelectorAll(m),d=0,h;h=p[d++];)r.highlightElement(h)}};var N=!1;r.fileHighlight=function(){N||(console.warn("Prism.fileHighlight is deprecated. Use `Prism.plugins.fileHighlight.highlight` instead."),N=!0),r.plugins.fileHighlight.highlight.apply(this,arguments)}}()})(uu);var pf=uu.exports;const ff=La(pf);Prism.languages.go=Prism.languages.extend("clike",{string:{pattern:/(^|[^\\])"(?:\\.|[^"\\\r\n])*"|`[^`]*`/,lookbehind:!0,greedy:!0},keyword:/\b(?:break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go(?:to)?|if|import|interface|map|package|range|return|select|struct|switch|type|var)\b/,boolean:/\b(?:_|false|iota|nil|true)\b/,number:[/\b0(?:b[01_]+|o[0-7_]+)i?\b/i,/\b0x(?:[a-f\d_]+(?:\.[a-f\d_]*)?|\.[a-f\d_]+)(?:p[+-]?\d+(?:_\d+)*)?i?(?!\w)/i,/(?:\b\d[\d_]*(?:\.[\d_]*)?|\B\.\d[\d_]*)(?:e[+-]?[\d_]+)?i?(?!\w)/i],operator:/[*\/%^!=]=?|\+[=+]?|-[=-]?|\|[=|]?|&(?:=|&|\^=?)?|>(?:>=?|=)?|<(?:<=?|=|-)?|:=|\.\.\./,builtin:/\b(?:append|bool|byte|cap|close|complex|complex(?:64|128)|copy|delete|error|float(?:32|64)|u?int(?:8|16|32|64)?|imag|len|make|new|panic|print(?:ln)?|real|recover|rune|string|uintptr)\b/});Prism.languages.insertBefore("go","string",{char:{pattern:/'(?:\\.|[^'\\\r\n]){0,10}'/,greedy:!0}});delete Prism.languages.go["class-name"];Prism.languages.python={comment:{pattern:/(^|[^\\])#.*/,lookbehind:!0,greedy:!0},"string-interpolation":{pattern:/(?:f|fr|rf)(?:("""|''')[\s\S]*?\1|("|')(?:\\.|(?!\2)[^\\\r\n])*\2)/i,greedy:!0,inside:{interpolation:{pattern:/((?:^|[^{])(?:\{\{)*)\{(?!\{)(?:[^{}]|\{(?!\{)(?:[^{}]|\{(?!\{)(?:[^{}])+\})+\})+\}/,lookbehind:!0,inside:{"format-spec":{pattern:/(:)[^:(){}]+(?=\}$)/,lookbehind:!0},"conversion-option":{pattern:/![sra](?=[:}]$)/,alias:"punctuation"},rest:null}},string:/[\s\S]+/}},"triple-quoted-string":{pattern:/(?:[rub]|br|rb)?("""|''')[\s\S]*?\1/i,greedy:!0,alias:"string"},string:{pattern:/(?:[rub]|br|rb)?("|')(?:\\.|(?!\1)[^\\\r\n])*\1/i,greedy:!0},function:{pattern:/((?:^|\s)def[ \t]+)[a-zA-Z_]\w*(?=\s*\()/g,lookbehind:!0},"class-name":{pattern:/(\bclass\s+)\w+/i,lookbehind:!0},decorator:{pattern:/(^[\t ]*)@\w+(?:\.\w+)*/m,lookbehind:!0,alias:["annotation","punctuation"],inside:{punctuation:/\./}},keyword:/\b(?:_(?=\s*:)|and|as|assert|async|await|break|case|class|continue|def|del|elif|else|except|exec|finally|for|from|global|if|import|in|is|lambda|match|nonlocal|not|or|pass|print|raise|return|try|while|with|yield)\b/,builtin:/\b(?:__import__|abs|all|any|apply|ascii|basestring|bin|bool|buffer|bytearray|bytes|callable|chr|classmethod|cmp|coerce|compile|complex|delattr|dict|dir|divmod|enumerate|eval|execfile|file|filter|float|format|frozenset|getattr|globals|hasattr|hash|help|hex|id|input|int|intern|isinstance|issubclass|iter|len|list|locals|long|map|max|memoryview|min|next|object|oct|open|ord|pow|property|range|raw_input|reduce|reload|repr|reversed|round|set|setattr|slice|sorted|staticmethod|str|sum|super|tuple|type|unichr|unicode|vars|xrange|zip)\b/,boolean:/\b(?:False|None|True)\b/,number:/\b0(?:b(?:_?[01])+|o(?:_?[0-7])+|x(?:_?[a-f0-9])+)\b|(?:\b\d+(?:_\d+)*(?:\.(?:\d+(?:_\d+)*)?)?|\B\.\d+(?:_\d+)*)(?:e[+-]?\d+(?:_\d+)*)?j?(?!\w)/i,operator:/[-+%=]=?|!=|:=|\*\*?=?|\/\/?=?|<[<=>]?|>[=>]?|[&|^~]/,punctuation:/[{}[\];(),.:]/};Prism.languages.python["string-interpolation"].inside.interpolation.inside.rest=Prism.languages.python;Prism.languages.py=Prism.languages.python;Prism.languages.javascript=Prism.languages.extend("clike",{"class-name":[Prism.languages.clike["class-name"],{pattern:/(^|[^$\w\xA0-\uFFFF])(?!\s)[_$A-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\.(?:constructor|prototype))/,lookbehind:!0}],keyword:[{pattern:/((?:^|\})\s*)catch\b/,lookbehind:!0},{pattern:/(^|[^.]|\.\.\.\s*)\b(?:as|assert(?=\s*\{)|async(?=\s*(?:function\b|\(|[$\w\xA0-\uFFFF]|$))|await|break|case|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally(?=\s*(?:\{|$))|for|from(?=\s*(?:['"]|$))|function|(?:get|set)(?=\s*(?:[#\[$\w\xA0-\uFFFF]|$))|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)\b/,lookbehind:!0}],function:/#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*(?:\.\s*(?:apply|bind|call)\s*)?\()/,number:{pattern:RegExp(/(^|[^\w$])/.source+"(?:"+(/NaN|Infinity/.source+"|"+/0[bB][01]+(?:_[01]+)*n?/.source+"|"+/0[oO][0-7]+(?:_[0-7]+)*n?/.source+"|"+/0[xX][\dA-Fa-f]+(?:_[\dA-Fa-f]+)*n?/.source+"|"+/\d+(?:_\d+)*n/.source+"|"+/(?:\d+(?:_\d+)*(?:\.(?:\d+(?:_\d+)*)?)?|\.\d+(?:_\d+)*)(?:[Ee][+-]?\d+(?:_\d+)*)?/.source)+")"+/(?![\w$])/.source),lookbehind:!0},operator:/--|\+\+|\*\*=?|=>|&&=?|\|\|=?|[!=]==|<<=?|>>>?=?|[-+*/%&|^!=<>]=?|\.{3}|\?\?=?|\?\.?|[~:]/});Prism.languages.javascript["class-name"][0].pattern=/(\b(?:class|extends|implements|instanceof|interface|new)\s+)[\w.\\]+/;Prism.languages.insertBefore("javascript","keyword",{regex:{pattern:RegExp(/((?:^|[^$\w\xA0-\uFFFF."'\])\s]|\b(?:return|yield))\s*)/.source+/\//.source+"(?:"+/(?:\[(?:[^\]\\\r\n]|\\.)*\]|\\.|[^/\\\[\r\n])+\/[dgimyus]{0,7}/.source+"|"+/(?:\[(?:[^[\]\\\r\n]|\\.|\[(?:[^[\]\\\r\n]|\\.|\[(?:[^[\]\\\r\n]|\\.)*\])*\])*\]|\\.|[^/\\\[\r\n])+\/[dgimyus]{0,7}v[dgimyus]{0,7}/.source+")"+/(?=(?:\s|\/\*(?:[^*]|\*(?!\/))*\*\/)*(?:$|[\r\n,.;:})\]]|\/\/))/.source),lookbehind:!0,greedy:!0,inside:{"regex-source":{pattern:/^(\/)[\s\S]+(?=\/[a-z]*$)/,lookbehind:!0,alias:"language-regex",inside:Prism.languages.regex},"regex-delimiter":/^\/|\/$/,"regex-flags":/^[a-z]+$/}},"function-variable":{pattern:/#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*[=:]\s*(?:async\s*)?(?:\bfunction\b|(?:\((?:[^()]|\([^()]*\))*\)|(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)\s*=>))/,alias:"function"},parameter:[{pattern:/(function(?:\s+(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)?\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\))/,lookbehind:!0,inside:Prism.languages.javascript},{pattern:/(^|[^$\w\xA0-\uFFFF])(?!\s)[_$a-z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*=>)/i,lookbehind:!0,inside:Prism.languages.javascript},{pattern:/(\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*=>)/,lookbehind:!0,inside:Prism.languages.javascript},{pattern:/((?:\b|\s|^)(?!(?:as|async|await|break|case|catch|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally|for|from|function|get|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|set|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)(?![$\w\xA0-\uFFFF]))(?:(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*\s*)\(\s*|\]\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*\{)/,lookbehind:!0,inside:Prism.languages.javascript}],constant:/\b[A-Z](?:[A-Z_]|\dx?)*\b/});Prism.languages.insertBefore("javascript","string",{hashbang:{pattern:/^#!.*/,greedy:!0,alias:"comment"},"template-string":{pattern:/`(?:\\[\s\S]|\$\{(?:[^{}]|\{(?:[^{}]|\{[^}]*\})*\})+\}|(?!\$\{)[^\\`])*`/,greedy:!0,inside:{"template-punctuation":{pattern:/^`|`$/,alias:"string"},interpolation:{pattern:/((?:^|[^\\])(?:\\{2})*)\$\{(?:[^{}]|\{(?:[^{}]|\{[^}]*\})*\})+\}/,lookbehind:!0,inside:{"interpolation-punctuation":{pattern:/^\$\{|\}$/,alias:"punctuation"},rest:Prism.languages.javascript}},string:/[\s\S]+/}},"string-property":{pattern:/((?:^|[,{])[ \t]*)(["'])(?:\\(?:\r\n|[\s\S])|(?!\2)[^\\\r\n])*\2(?=\s*:)/m,lookbehind:!0,greedy:!0,alias:"property"}});Prism.languages.insertBefore("javascript","operator",{"literal-property":{pattern:/((?:^|[,{])[ \t]*)(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*:)/m,lookbehind:!0,alias:"property"}});Prism.languages.markup&&(Prism.languages.markup.tag.addInlined("script","javascript"),Prism.languages.markup.tag.addAttribute(/on(?:abort|blur|change|click|composition(?:end|start|update)|dblclick|error|focus(?:in|out)?|key(?:down|up)|load|mouse(?:down|enter|leave|move|out|over|up)|reset|resize|scroll|select|slotchange|submit|unload|wheel)/.source,"javascript"));Prism.languages.js=Prism.languages.javascript;(function(n){n.languages.typescript=n.languages.extend("javascript",{"class-name":{pattern:/(\b(?:class|extends|implements|instanceof|interface|new|type)\s+)(?!keyof\b)(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?:\s*<(?:[^<>]|<(?:[^<>]|<[^<>]*>)*>)*>)?/,lookbehind:!0,greedy:!0,inside:null},builtin:/\b(?:Array|Function|Promise|any|boolean|console|never|number|string|symbol|unknown)\b/}),n.languages.typescript.keyword.push(/\b(?:abstract|declare|is|keyof|readonly|require)\b/,/\b(?:asserts|infer|interface|module|namespace|type)\b(?=\s*(?:[{_$a-zA-Z\xA0-\uFFFF]|$))/,/\btype\b(?=\s*(?:[\{*]|$))/),delete n.languages.typescript.parameter,delete n.languages.typescript["literal-property"];var t=n.languages.extend("typescript",{});delete t["class-name"],n.languages.typescript["class-name"].inside=t,n.languages.insertBefore("typescript","function",{decorator:{pattern:/@[$\w\xA0-\uFFFF]+/,inside:{at:{pattern:/^@/,alias:"operator"},function:/^[\s\S]+/}},"generic-function":{pattern:/#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*\s*<(?:[^<>]|<(?:[^<>]|<[^<>]*>)*>)*>(?=\s*\()/,greedy:!0,inside:{function:/^#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*/,generic:{pattern:/<[\s\S]+/,alias:"class-name",inside:t}}}}),n.languages.ts=n.languages.typescript})(Prism);(function(n){for(var t=/\/\*(?:[^*/]|\*(?!\/)|\/(?!\*)|<self>)*\*\//.source,r=0;r<2;r++)t=t.replace(/<self>/g,function(){return t});t=t.replace(/<self>/g,function(){return/[^\s\S]/.source}),n.languages.rust={comment:[{pattern:RegExp(/(^|[^\\])/.source+t),lookbehind:!0,greedy:!0},{pattern:/(^|[^\\:])\/\/.*/,lookbehind:!0,greedy:!0}],string:{pattern:/b?"(?:\\[\s\S]|[^\\"])*"|b?r(#*)"(?:[^"]|"(?!\1))*"\1/,greedy:!0},char:{pattern:/b?'(?:\\(?:x[0-7][\da-fA-F]|u\{(?:[\da-fA-F]_*){1,6}\}|.)|[^\\\r\n\t'])'/,greedy:!0},attribute:{pattern:/#!?\[(?:[^\[\]"]|"(?:\\[\s\S]|[^\\"])*")*\]/,greedy:!0,alias:"attr-name",inside:{string:null}},"closure-params":{pattern:/([=(,:]\s*|\bmove\s*)\|[^|]*\||\|[^|]*\|(?=\s*(?:\{|->))/,lookbehind:!0,greedy:!0,inside:{"closure-punctuation":{pattern:/^\||\|$/,alias:"punctuation"},rest:null}},"lifetime-annotation":{pattern:/'\w+/,alias:"symbol"},"fragment-specifier":{pattern:/(\$\w+:)[a-z]+/,lookbehind:!0,alias:"punctuation"},variable:/\$\w+/,"function-definition":{pattern:/(\bfn\s+)\w+/,lookbehind:!0,alias:"function"},"type-definition":{pattern:/(\b(?:enum|struct|trait|type|union)\s+)\w+/,lookbehind:!0,alias:"class-name"},"module-declaration":[{pattern:/(\b(?:crate|mod)\s+)[a-z][a-z_\d]*/,lookbehind:!0,alias:"namespace"},{pattern:/(\b(?:crate|self|super)\s*)::\s*[a-z][a-z_\d]*\b(?:\s*::(?:\s*[a-z][a-z_\d]*\s*::)*)?/,lookbehind:!0,alias:"namespace",inside:{punctuation:/::/}}],keyword:[/\b(?:Self|abstract|as|async|await|become|box|break|const|continue|crate|do|dyn|else|enum|extern|final|fn|for|if|impl|in|let|loop|macro|match|mod|move|mut|override|priv|pub|ref|return|self|static|struct|super|trait|try|type|typeof|union|unsafe|unsized|use|virtual|where|while|yield)\b/,/\b(?:bool|char|f(?:32|64)|[ui](?:8|16|32|64|128|size)|str)\b/],function:/\b[a-z_]\w*(?=\s*(?:::\s*<|\())/,macro:{pattern:/\b\w+!/,alias:"property"},constant:/\b[A-Z_][A-Z_\d]+\b/,"class-name":/\b[A-Z]\w*\b/,namespace:{pattern:/(?:\b[a-z][a-z_\d]*\s*::\s*)*\b[a-z][a-z_\d]*\s*::(?!\s*<)/,inside:{punctuation:/::/}},number:/\b(?:0x[\dA-Fa-f](?:_?[\dA-Fa-f])*|0o[0-7](?:_?[0-7])*|0b[01](?:_?[01])*|(?:(?:\d(?:_?\d)*)?\.)?\d(?:_?\d)*(?:[Ee][+-]?\d+)?)(?:_?(?:f32|f64|[iu](?:8|16|32|64|size)?))?\b/,boolean:/\b(?:false|true)\b/,punctuation:/->|\.\.=|\.{1,3}|::|[{}[\];(),:]/,operator:/[-+*\/%!^]=?|=[=>]?|&[&=]?|\|[|=]?|<<?=?|>>?=?|[@?]/},n.languages.rust["closure-params"].inside.rest=n.languages.rust,n.languages.rust.attribute.inside.string=n.languages.rust.string})(Prism);Prism.languages.sql={comment:{pattern:/(^|[^\\])(?:\/\*[\s\S]*?\*\/|(?:--|\/\/|#).*)/,lookbehind:!0},variable:[{pattern:/@(["'`])(?:\\[\s\S]|(?!\1)[^\\])+\1/,greedy:!0},/@[\w.$]+/],string:{pattern:/(^|[^@\\])("|')(?:\\[\s\S]|(?!\2)[^\\]|\2\2)*\2/,greedy:!0,lookbehind:!0},identifier:{pattern:/(^|[^@\\])`(?:\\[\s\S]|[^`\\]|``)*`/,greedy:!0,lookbehind:!0,inside:{punctuation:/^`|`$/}},function:/\b(?:AVG|COUNT|FIRST|FORMAT|LAST|LCASE|LEN|MAX|MID|MIN|MOD|NOW|ROUND|SUM|UCASE)(?=\s*\()/i,keyword:/\b(?:ACTION|ADD|AFTER|ALGORITHM|ALL|ALTER|ANALYZE|ANY|APPLY|AS|ASC|AUTHORIZATION|AUTO_INCREMENT|BACKUP|BDB|BEGIN|BERKELEYDB|BIGINT|BINARY|BIT|BLOB|BOOL|BOOLEAN|BREAK|BROWSE|BTREE|BULK|BY|CALL|CASCADED?|CASE|CHAIN|CHAR(?:ACTER|SET)?|CHECK(?:POINT)?|CLOSE|CLUSTERED|COALESCE|COLLATE|COLUMNS?|COMMENT|COMMIT(?:TED)?|COMPUTE|CONNECT|CONSISTENT|CONSTRAINT|CONTAINS(?:TABLE)?|CONTINUE|CONVERT|CREATE|CROSS|CURRENT(?:_DATE|_TIME|_TIMESTAMP|_USER)?|CURSOR|CYCLE|DATA(?:BASES?)?|DATE(?:TIME)?|DAY|DBCC|DEALLOCATE|DEC|DECIMAL|DECLARE|DEFAULT|DEFINER|DELAYED|DELETE|DELIMITERS?|DENY|DESC|DESCRIBE|DETERMINISTIC|DISABLE|DISCARD|DISK|DISTINCT|DISTINCTROW|DISTRIBUTED|DO|DOUBLE|DROP|DUMMY|DUMP(?:FILE)?|DUPLICATE|ELSE(?:IF)?|ENABLE|ENCLOSED|END|ENGINE|ENUM|ERRLVL|ERRORS|ESCAPED?|EXCEPT|EXEC(?:UTE)?|EXISTS|EXIT|EXPLAIN|EXTENDED|FETCH|FIELDS|FILE|FILLFACTOR|FIRST|FIXED|FLOAT|FOLLOWING|FOR(?: EACH ROW)?|FORCE|FOREIGN|FREETEXT(?:TABLE)?|FROM|FULL|FUNCTION|GEOMETRY(?:COLLECTION)?|GLOBAL|GOTO|GRANT|GROUP|HANDLER|HASH|HAVING|HOLDLOCK|HOUR|IDENTITY(?:COL|_INSERT)?|IF|IGNORE|IMPORT|INDEX|INFILE|INNER|INNODB|INOUT|INSERT|INT|INTEGER|INTERSECT|INTERVAL|INTO|INVOKER|ISOLATION|ITERATE|JOIN|KEYS?|KILL|LANGUAGE|LAST|LEAVE|LEFT|LEVEL|LIMIT|LINENO|LINES|LINESTRING|LOAD|LOCAL|LOCK|LONG(?:BLOB|TEXT)|LOOP|MATCH(?:ED)?|MEDIUM(?:BLOB|INT|TEXT)|MERGE|MIDDLEINT|MINUTE|MODE|MODIFIES|MODIFY|MONTH|MULTI(?:LINESTRING|POINT|POLYGON)|NATIONAL|NATURAL|NCHAR|NEXT|NO|NONCLUSTERED|NULLIF|NUMERIC|OFF?|OFFSETS?|ON|OPEN(?:DATASOURCE|QUERY|ROWSET)?|OPTIMIZE|OPTION(?:ALLY)?|ORDER|OUT(?:ER|FILE)?|OVER|PARTIAL|PARTITION|PERCENT|PIVOT|PLAN|POINT|POLYGON|PRECEDING|PRECISION|PREPARE|PREV|PRIMARY|PRINT|PRIVILEGES|PROC(?:EDURE)?|PUBLIC|PURGE|QUICK|RAISERROR|READS?|REAL|RECONFIGURE|REFERENCES|RELEASE|RENAME|REPEAT(?:ABLE)?|REPLACE|REPLICATION|REQUIRE|RESIGNAL|RESTORE|RESTRICT|RETURN(?:ING|S)?|REVOKE|RIGHT|ROLLBACK|ROUTINE|ROW(?:COUNT|GUIDCOL|S)?|RTREE|RULE|SAVE(?:POINT)?|SCHEMA|SECOND|SELECT|SERIAL(?:IZABLE)?|SESSION(?:_USER)?|SET(?:USER)?|SHARE|SHOW|SHUTDOWN|SIMPLE|SMALLINT|SNAPSHOT|SOME|SONAME|SQL|START(?:ING)?|STATISTICS|STATUS|STRIPED|SYSTEM_USER|TABLES?|TABLESPACE|TEMP(?:ORARY|TABLE)?|TERMINATED|TEXT(?:SIZE)?|THEN|TIME(?:STAMP)?|TINY(?:BLOB|INT|TEXT)|TOP?|TRAN(?:SACTIONS?)?|TRIGGER|TRUNCATE|TSEQUAL|TYPES?|UNBOUNDED|UNCOMMITTED|UNDEFINED|UNION|UNIQUE|UNLOCK|UNPIVOT|UNSIGNED|UPDATE(?:TEXT)?|USAGE|USE|USER|USING|VALUES?|VAR(?:BINARY|CHAR|CHARACTER|YING)|VIEW|WAITFOR|WARNINGS|WHEN|WHERE|WHILE|WITH(?: ROLLUP|IN)?|WORK|WRITE(?:TEXT)?|YEAR)\b/i,boolean:/\b(?:FALSE|NULL|TRUE)\b/i,number:/\b0x[\da-f]+\b|\b\d+(?:\.\d*)?|\B\.\d+\b/i,operator:/[-+*\/=%^~]|&&?|\|\|?|!=?|<(?:=>?|<|>)?|>[>=]?|\b(?:AND|BETWEEN|DIV|ILIKE|IN|IS|LIKE|NOT|OR|REGEXP|RLIKE|SOUNDS LIKE|XOR)\b/i,punctuation:/[;[\]()`,.]/};Prism.languages.json={property:{pattern:/(^|[^\\])"(?:\\.|[^\\"\r\n])*"(?=\s*:)/,lookbehind:!0,greedy:!0},string:{pattern:/(^|[^\\])"(?:\\.|[^\\"\r\n])*"(?!\s*:)/,lookbehind:!0,greedy:!0},comment:{pattern:/\/\/.*|\/\*[\s\S]*?(?:\*\/|$)/,greedy:!0},number:/-?\b\d+(?:\.\d+)?(?:e[+-]?\d+)?\b/i,punctuation:/[{}[\],]/,operator:/:/,boolean:/\b(?:false|true)\b/,null:{pattern:/\bnull\b/,alias:"keyword"}};Prism.languages.webmanifest=Prism.languages.json;(function(n){var t=/[*&][^\s[\]{},]+/,r=/!(?:<[\w\-%#;/?:@&=+$,.!~*'()[\]]+>|(?:[a-zA-Z\d-]*!)?[\w\-%#;/?:@&=+$.~*'()]+)?/,s="(?:"+r.source+"(?:[ 	]+"+t.source+")?|"+t.source+"(?:[ 	]+"+r.source+")?)",l=/(?:[^\s\x00-\x08\x0e-\x1f!"#%&'*,\-:>?@[\]`{|}\x7f-\x84\x86-\x9f\ud800-\udfff\ufffe\uffff]|[?:-]<PLAIN>)(?:[ \t]*(?:(?![#:])<PLAIN>|:<PLAIN>))*/.source.replace(/<PLAIN>/g,function(){return/[^\s\x00-\x08\x0e-\x1f,[\]{}\x7f-\x84\x86-\x9f\ud800-\udfff\ufffe\uffff]/.source}),i=/"(?:[^"\\\r\n]|\\.)*"|'(?:[^'\\\r\n]|\\.)*'/.source;function o(a,c){c=(c||"").replace(/m/g,"")+"m";var u=/([:\-,[{]\s*(?:\s<<prop>>[ \t]+)?)(?:<<value>>)(?=[ \t]*(?:$|,|\]|\}|(?:[\r\n]\s*)?#))/.source.replace(/<<prop>>/g,function(){return s}).replace(/<<value>>/g,function(){return a});return RegExp(u,c)}n.languages.yaml={scalar:{pattern:RegExp(/([\-:]\s*(?:\s<<prop>>[ \t]+)?[|>])[ \t]*(?:((?:\r?\n|\r)[ \t]+)\S[^\r\n]*(?:\2[^\r\n]+)*)/.source.replace(/<<prop>>/g,function(){return s})),lookbehind:!0,alias:"string"},comment:/#.*/,key:{pattern:RegExp(/((?:^|[:\-,[{\r\n?])[ \t]*(?:<<prop>>[ \t]+)?)<<key>>(?=\s*:\s)/.source.replace(/<<prop>>/g,function(){return s}).replace(/<<key>>/g,function(){return"(?:"+l+"|"+i+")"})),lookbehind:!0,greedy:!0,alias:"atrule"},directive:{pattern:/(^[ \t]*)%.+/m,lookbehind:!0,alias:"important"},datetime:{pattern:o(/\d{4}-\d\d?-\d\d?(?:[tT]|[ \t]+)\d\d?:\d{2}:\d{2}(?:\.\d*)?(?:[ \t]*(?:Z|[-+]\d\d?(?::\d{2})?))?|\d{4}-\d{2}-\d{2}|\d\d?:\d{2}(?::\d{2}(?:\.\d*)?)?/.source),lookbehind:!0,alias:"number"},boolean:{pattern:o(/false|true/.source,"i"),lookbehind:!0,alias:"important"},null:{pattern:o(/null|~/.source,"i"),lookbehind:!0,alias:"important"},string:{pattern:o(i),lookbehind:!0,greedy:!0},number:{pattern:o(/[+-]?(?:0x[\da-f]+|0o[0-7]+|(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?|\.inf|\.nan)/.source,"i"),lookbehind:!0},tag:r,important:t,punctuation:/---|[:[\]{}\-,|>?]|\.\.\./},n.languages.yml=n.languages.yaml})(Prism);function hn({code:n,language:t="go",collapsible:r=!1}){const s=y.useRef(null),[l,i]=y.useState(!1),[o,a]=y.useState(!1),[c,u]=y.useState(0);y.useEffect(()=>{if(s.current){ff.highlightElement(s.current);const g=n.split(`
`).length;u(g),console.log("CodeBlock: Lines:",g,"Collapse?:",g>3)}},[n]);const x=async()=>{try{if(console.log("Attempting to copy code:",{codeLength:n.length,code:n.substring(0,100)}),navigator.clipboard&&navigator.clipboard.writeText)await navigator.clipboard.writeText(n),console.log("Successfully copied to clipboard"),a(!0),setTimeout(()=>a(!1),2e3);else{const g=document.createElement("textarea");g.value=n,g.style.position="fixed",g.style.left="-999999px",document.body.appendChild(g),g.focus(),g.select();const w=document.execCommand("copy");document.body.removeChild(g),w?(console.log("Successfully copied to clipboard (fallback)"),a(!0),setTimeout(()=>a(!1),2e3)):console.error("Fallback copy failed")}}catch(g){console.error("Failed to copy:",g)}},m=r&&c>3,f=t==="go"?"code-go":"code-block",k=`language-${t}`,N=`${f} ${m&&!l?"collapsed":""}`;return console.log("CodeBlock render:",{lineCount:c,shouldCollapse:m,isExpanded:l,preClassName:N}),e.jsxs("div",{className:"code-block-container",children:[e.jsxs("div",{className:"code-block-header",children:[e.jsx("div",{className:"code-block-info",children:m&&e.jsxs("button",{className:"code-block-toggle",onClick:()=>i(!l),"aria-label":l?"Collapse code":"Expand code",title:l?"Collapse":"Expand",children:[e.jsx("svg",{width:"16",height:"16",viewBox:"0 0 16 16",fill:"currentColor",children:l?e.jsx("path",{d:"M2 5l6 6 6-6H2z"}):e.jsx("path",{d:"M6 2v12l6-6-6-6z"})}),e.jsx("span",{className:"code-block-line-count",children:l?"Collapse":`Show all (${c} lines)`})]})}),e.jsxs("button",{className:"code-copy-btn",onClick:x,"aria-label":"Copy code",title:"Copy to clipboard",children:[e.jsxs("svg",{width:"16",height:"16",viewBox:"0 0 16 16",fill:"currentColor",children:[e.jsx("path",{d:"M4 1.5H3a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2V3.5a2 2 0 0 0-2-2h-1v-1a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v1zm5-1v1H6v-1h3z"}),e.jsx("path",{d:"M4 5.5a.5.5 0 0 1 .5-.5h7a.5.5 0 0 1 .5.5v9a.5.5 0 0 1-.5.5H4.5a.5.5 0 0 1-.5-.5v-9z"})]}),e.jsx("span",{className:`copy-feedback ${o?"show":""}`,children:o?"Copied!":"Copy"})]})]}),e.jsx("pre",{className:N,children:e.jsx("code",{ref:s,className:k,children:n})})]})}function xf(n){const t=[],r=/```(\w*)\n?([\s\S]*?)```/g;let s=0,l,i=0;for(;(l=r.exec(n))!==null;){if(l.index>s){const c=n.slice(s,l.index);t.push(e.jsx("span",{children:Ma(c)},i++))}const o=l[1]||"text",a=l[2].trim();t.push(e.jsx("div",{className:"chat-code-block-wrapper",children:e.jsx(hn,{code:a,language:o,collapsible:!0})},i++)),s=l.index+l[0].length}if(s<n.length){const o=n.slice(s);t.push(e.jsx("span",{children:Ma(o)},i++))}return t}function Ma(n){const t=[],r=/`([^`]+)`/g;let s=0,l,i=0;for(;(l=r.exec(n))!==null;)l.index>s&&t.push(e.jsx("span",{children:n.slice(s,l.index)},i++)),t.push(e.jsx("code",{children:l[1]},i++)),s=l.index+l[0].length;return s<n.length&&t.push(e.jsx("span",{children:n.slice(s)},i++)),t}function jf(){var z,Ie;const{models:n,loading:t,loadModels:r}=Bs(),[s,l]=y.useState(""),[i,o]=y.useState([]),[a,c]=y.useState(""),[u,x]=y.useState(!1),[m,f]=y.useState(null),[k,N]=y.useState(!1),[g,w]=y.useState(2048),[p,d]=y.useState(.7),[h,j]=y.useState(.9),[v,b]=y.useState(40),E=y.useRef(null),S=y.useRef(null);y.useEffect(()=>{r()},[r]),y.useEffect(()=>{n!=null&&n.data&&n.data.length>0&&!s&&l(n.data[0].id)},[n,s]),y.useEffect(()=>{var I;(I=E.current)==null||I.scrollIntoView({behavior:"smooth"})},[i]);const C=I=>{if(I.preventDefault(),!a.trim()||!s||u)return;const L={role:"user",content:a.trim()};o(F=>[...F,L]),c(""),f(null),x(!0);const G=[...i.map(F=>({role:F.role,content:F.content})),{role:"user",content:a.trim()}];let R="",O="",M,B=[];o(F=>[...F,{role:"assistant",content:"",reasoning:""}]),S.current=fe.streamChat({model:s,messages:G,max_tokens:g,temperature:p,top_p:h,top_k:v},F=>{var se,Oe,je,Me;const ne=(se=F.choices)==null?void 0:se[0];(Oe=ne==null?void 0:ne.delta)!=null&&Oe.content&&(R+=ne.delta.content),(je=ne==null?void 0:ne.delta)!=null&&je.reasoning&&(O+=ne.delta.reasoning),(Me=ne==null?void 0:ne.delta)!=null&&Me.tool_calls&&ne.delta.tool_calls.length>0&&(B=[...B,...ne.delta.tool_calls]),F.usage&&(M=F.usage),o(Rt=>{const Xn=[...Rt];return Xn[Xn.length-1]={role:"assistant",content:R,reasoning:O,usage:M,toolCalls:B.length?B:void 0},Xn})},F=>{f(F),x(!1)},()=>{x(!1)})},_=()=>{S.current&&(S.current(),S.current=null,x(!1))},U=()=>{o([]),f(null)},A=I=>{I.key==="Enter"&&!I.shiftKey&&(I.preventDefault(),C(I))};return e.jsxs("div",{className:"chat-container",children:[e.jsxs("div",{className:"chat-header",children:[e.jsxs("div",{className:"chat-header-left",children:[e.jsx("h2",{children:"Run"}),e.jsxs("select",{value:s,onChange:I=>l(I.target.value),disabled:t||u,className:"chat-model-select",children:[t&&e.jsx("option",{children:"Loading models..."}),!t&&((z=n==null?void 0:n.data)==null?void 0:z.length)===0&&e.jsx("option",{children:"No models available"}),(Ie=n==null?void 0:n.data)==null?void 0:Ie.map(I=>e.jsx("option",{value:I.id,children:I.id},I.id))]})]}),e.jsxs("div",{className:"chat-header-right",children:[e.jsx("button",{className:"btn btn-secondary btn-sm",onClick:()=>N(!k),children:"Settings"}),e.jsx("button",{className:"btn btn-secondary btn-sm",onClick:U,disabled:u||i.length===0,children:"Clear"})]})]}),k&&e.jsxs("div",{className:"chat-settings",children:[e.jsxs("div",{className:"chat-setting",children:[e.jsx("label",{children:"Max Tokens"}),e.jsx("input",{type:"number",value:g,onChange:I=>w(Number(I.target.value)),min:1,max:32768})]}),e.jsxs("div",{className:"chat-setting",children:[e.jsx("label",{children:"Temperature"}),e.jsx("input",{type:"number",value:p,onChange:I=>d(Number(I.target.value)),min:0,max:2,step:.1})]}),e.jsxs("div",{className:"chat-setting",children:[e.jsx("label",{children:"Top P"}),e.jsx("input",{type:"number",value:h,onChange:I=>j(Number(I.target.value)),min:0,max:1,step:.05})]}),e.jsxs("div",{className:"chat-setting",children:[e.jsx("label",{children:"Top K"}),e.jsx("input",{type:"number",value:v,onChange:I=>b(Number(I.target.value)),min:1,max:100})]})]}),m&&e.jsx("div",{className:"alert alert-error",children:m}),e.jsxs("div",{className:"chat-messages",children:[i.length===0&&e.jsxs("div",{className:"chat-empty",children:[e.jsx("p",{children:"Select a model and start chatting"}),e.jsx("p",{className:"chat-empty-hint",children:"Type a message below to begin"})]}),i.map((I,L)=>e.jsxs("div",{className:`chat-message chat-message-${I.role}`,children:[e.jsx("div",{className:"chat-message-header",children:I.role==="user"?"USER":"MODEL"}),I.reasoning&&e.jsx("div",{className:"chat-message-reasoning",children:I.reasoning}),e.jsx("div",{className:"chat-message-content",children:I.content?xf(I.content):u&&L===i.length-1?"...":""}),I.toolCalls&&I.toolCalls.length>0&&e.jsx("div",{className:"chat-message-tool-calls",children:I.toolCalls.map(G=>e.jsxs("div",{className:"chat-tool-call",children:["Tool call ",G.id,": ",G.function.name,"(",G.function.arguments,")"]},G.id))}),I.usage&&e.jsxs("div",{className:"chat-message-usage",children:["Input: ",I.usage.prompt_tokens," | Reasoning: ",I.usage.reasoning_tokens," | Completion: ",I.usage.completion_tokens," | Output: ",I.usage.output_tokens," | TPS: ",I.usage.tokens_per_second.toFixed(2)]})]},L)),e.jsx("div",{ref:E})]}),e.jsxs("form",{onSubmit:C,className:"chat-input-form",children:[e.jsx("textarea",{value:a,onChange:I=>c(I.target.value),onKeyDown:A,placeholder:"Type your message... (Enter to send, Shift+Enter for new line)",disabled:u||!s,className:"chat-input",rows:3}),e.jsx("div",{className:"chat-input-actions",children:u?e.jsx("button",{type:"button",className:"btn btn-danger",onClick:_,children:"Stop"}):e.jsx("button",{type:"submit",className:"btn btn-primary",disabled:!a.trim()||!s,children:"Send"})})]})]})}function gf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"SDK Documentation"}),e.jsx("p",{children:"Documentation for the Kronk SDK"})]}),e.jsx("div",{className:"card",children:e.jsxs("div",{className:"empty-state",children:[e.jsx("h3",{children:"🚧 Under Construction"}),e.jsx("p",{children:"SDK documentation is coming soon."})]})})]})}function vf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Kronk Package"}),e.jsx("p",{children:"Package kronk provides support for working with models using llama.cpp via yzma."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",children:[e.jsx("h3",{children:"Import"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'import "github.com/ardanlabs/kronk/sdk/kronk"'})})]}),e.jsxs("div",{className:"card",id:"functions",children:[e.jsx("h3",{children:"Functions"}),e.jsxs("div",{className:"doc-section",id:"func-init",children:[e.jsx("h4",{children:"Init"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func Init(opts ...InitOption) error"})}),e.jsx("p",{className:"doc-description",children:"Init initializes the Kronk backend support."})]}),e.jsxs("div",{className:"doc-section",id:"func-setfmtloggertraceid",children:[e.jsx("h4",{children:"SetFmtLoggerTraceID"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func SetFmtLoggerTraceID(ctx context.Context, traceID string) context.Context"})}),e.jsx("p",{className:"doc-description",children:"SetFmtLoggerTraceID allows you to set a trace id in the content that can be part of the output of the FmtLogger."})]}),e.jsxs("div",{className:"doc-section",id:"func-new",children:[e.jsx("h4",{children:"New"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func New(cfg model.Config, opts ...Option) (*Kronk, error)"})}),e.jsx("p",{className:"doc-description",children:"New provides the ability to use models in a concurrently safe way."})]})]}),e.jsxs("div",{className:"card",id:"types",children:[e.jsx("h3",{children:"Types"}),e.jsxs("div",{className:"doc-section",id:"type-incompletedetail",children:[e.jsx("h4",{children:"IncompleteDetail"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type IncompleteDetail struct {\n	Reason string `json:"reason"`\n}'})}),e.jsx("p",{className:"doc-description",children:"IncompleteDetail provides details about why a response is incomplete."})]}),e.jsxs("div",{className:"doc-section",id:"type-initoption",children:[e.jsx("h4",{children:"InitOption"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type InitOption func(*initOptions)"})}),e.jsx("p",{className:"doc-description",children:"InitOption represents options for configuring Init."})]}),e.jsxs("div",{className:"doc-section",id:"type-inputtokensdetails",children:[e.jsx("h4",{children:"InputTokensDetails"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type InputTokensDetails struct {\n	CachedTokens int `json:"cached_tokens"`\n}'})}),e.jsx("p",{className:"doc-description",children:"InputTokensDetails provides breakdown of input tokens."})]}),e.jsxs("div",{className:"doc-section",id:"type-kronk",children:[e.jsx("h4",{children:"Kronk"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`type Kronk struct {
	// Has unexported fields.
}`})}),e.jsx("p",{className:"doc-description",children:"Kronk provides a concurrently safe api for using llama.cpp to access models."})]}),e.jsxs("div",{className:"doc-section",id:"type-loglevel",children:[e.jsx("h4",{children:"LogLevel"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type LogLevel int"})}),e.jsx("p",{className:"doc-description",children:"LogLevel represents the logging level."})]}),e.jsxs("div",{className:"doc-section",id:"type-logger",children:[e.jsx("h4",{children:"Logger"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type Logger func(ctx context.Context, msg string, args ...any)"})}),e.jsx("p",{className:"doc-description",children:"Logger provides a function for logging messages from different APIs."})]}),e.jsxs("div",{className:"doc-section",id:"type-option",children:[e.jsx("h4",{children:"Option"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type Option func(*options)"})}),e.jsx("p",{className:"doc-description",children:"Option represents options for configuring Kronk."})]}),e.jsxs("div",{className:"doc-section",id:"type-outputtokensdetails",children:[e.jsx("h4",{children:"OutputTokensDetails"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type OutputTokensDetails struct {\n	ReasoningTokens int `json:"reasoning_tokens"`\n}'})}),e.jsx("p",{className:"doc-description",children:"OutputTokensDetails provides breakdown of output tokens."})]}),e.jsxs("div",{className:"doc-section",id:"type-responsecontentitem",children:[e.jsx("h4",{children:"ResponseContentItem"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseContentItem struct {\n	Type        string   `json:"type"`\n	Text        string   `json:"text"`\n	Annotations []string `json:"annotations"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseContentItem represents a content item within an output message."})]}),e.jsxs("div",{className:"doc-section",id:"type-responseerror",children:[e.jsx("h4",{children:"ResponseError"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseError struct {\n	Code    string `json:"code"`\n	Message string `json:"message"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseError represents an error in the response."})]}),e.jsxs("div",{className:"doc-section",id:"type-responseformattype",children:[e.jsx("h4",{children:"ResponseFormatType"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseFormatType struct {\n	Type string `json:"type"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseFormatType specifies the format type."})]}),e.jsxs("div",{className:"doc-section",id:"type-responseoutputitem",children:[e.jsx("h4",{children:"ResponseOutputItem"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseOutputItem struct {\n	Type      string                `json:"type"`\n	ID        string                `json:"id"`\n	Status    string                `json:"status,omitempty"`\n	Role      string                `json:"role,omitempty"`\n	Content   []ResponseContentItem `json:"content,omitempty"`\n	CallID    string                `json:"call_id,omitempty"`\n	Name      string                `json:"name,omitempty"`\n	Arguments string                `json:"arguments,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:'ResponseOutputItem represents an item in the output array. For type="message": ID, Status, Role, Content are used. For type="function_call": ID, Status, CallID, Name, Arguments are used.'})]}),e.jsxs("div",{className:"doc-section",id:"type-responsereasoning",children:[e.jsx("h4",{children:"ResponseReasoning"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseReasoning struct {\n	Effort  *string `json:"effort"`\n	Summary *string `json:"summary"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseReasoning contains reasoning configuration/output."})]}),e.jsxs("div",{className:"doc-section",id:"type-responseresponse",children:[e.jsx("h4",{children:"ResponseResponse"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseResponse struct {\n	ID               string                 `json:"id"`\n	Object           string                 `json:"object"`\n	CreatedAt        int64                  `json:"created_at"`\n	Status           string                 `json:"status"`\n	CompletedAt      *int64                 `json:"completed_at"`\n	Error            *ResponseError         `json:"error"`\n	IncompleteDetail *IncompleteDetail      `json:"incomplete_details"`\n	Instructions     *string                `json:"instructions"`\n	MaxOutputTokens  *int                   `json:"max_output_tokens"`\n	Model            string                 `json:"model"`\n	Output           []ResponseOutputItem   `json:"output"`\n	ParallelToolCall bool                   `json:"parallel_tool_calls"`\n	PrevResponseID   *string                `json:"previous_response_id"`\n	Reasoning        ResponseReasoning      `json:"reasoning"`\n	Store            bool                   `json:"store"`\n	Temperature      float64                `json:"temperature"`\n	Text             ResponseTextFormat     `json:"text"`\n	ToolChoice       string                 `json:"tool_choice"`\n	Tools            []any                  `json:"tools"`\n	TopP             float64                `json:"top_p"`\n	Truncation       string                 `json:"truncation"`\n	Usage            ResponseUsage          `json:"usage"`\n	User             *string                `json:"user"`\n	Metadata         map[string]interface{} `json:"metadata"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseResponse represents the OpenAI Responses API response format."})]}),e.jsxs("div",{className:"doc-section",id:"type-responsestreamevent",children:[e.jsx("h4",{children:"ResponseStreamEvent"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseStreamEvent struct {\n	Type           string               `json:"type"`\n	SequenceNumber int                  `json:"sequence_number"`\n	Response       *ResponseResponse    `json:"response,omitempty"`\n	OutputIndex    *int                 `json:"output_index,omitempty"`\n	ContentIndex   *int                 `json:"content_index,omitempty"`\n	ItemID         string               `json:"item_id,omitempty"`\n	Item           *ResponseOutputItem  `json:"item,omitempty"`\n	Part           *ResponseContentItem `json:"part,omitempty"`\n	Delta          string               `json:"delta,omitempty"`\n	Text           string               `json:"text,omitempty"`\n	Arguments      string               `json:"arguments,omitempty"`\n	Name           string               `json:"name,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseStreamEvent represents a streaming event for the Responses API."})]}),e.jsxs("div",{className:"doc-section",id:"type-responsetextformat",children:[e.jsx("h4",{children:"ResponseTextFormat"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseTextFormat struct {\n	Format ResponseFormatType `json:"format"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseTextFormat specifies the text format configuration."})]}),e.jsxs("div",{className:"doc-section",id:"type-responseusage",children:[e.jsx("h4",{children:"ResponseUsage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseUsage struct {\n	InputTokens        int                 `json:"input_tokens"`\n	InputTokensDetails InputTokensDetails  `json:"input_tokens_details"`\n	OutputTokens       int                 `json:"output_tokens"`\n	OutputTokenDetail  OutputTokensDetails `json:"output_tokens_details"`\n	TotalTokens        int                 `json:"total_tokens"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseUsage contains token usage information."})]})]}),e.jsxs("div",{className:"card",id:"methods",children:[e.jsx("h3",{children:"Methods"}),e.jsxs("div",{className:"doc-section",id:"method-kronk-activestreams",children:[e.jsx("h4",{children:"Kronk.ActiveStreams"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) ActiveStreams() int"})}),e.jsx("p",{className:"doc-description",children:"ActiveStreams returns the number of active streams."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-chat",children:[e.jsx("h4",{children:"Kronk.Chat"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) Chat(ctx context.Context, d model.D) (model.ChatResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"Chat provides support to interact with an inference model. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-chatstreaming",children:[e.jsx("h4",{children:"Kronk.ChatStreaming"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) ChatStreaming(ctx context.Context, d model.D) (<-chan model.ChatResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"ChatStreaming provides support to interact with an inference model. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-chatstreaminghttp",children:[e.jsx("h4",{children:"Kronk.ChatStreamingHTTP"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (model.ChatResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"ChatStreamingHTTP provides http handler support for a chat/completions call. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-embeddings",children:[e.jsx("h4",{children:"Kronk.Embeddings"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) Embeddings(ctx context.Context, d model.D) (model.EmbedReponse, error)"})}),e.jsx("p",{className:"doc-description",children:'Embeddings provides support to interact with an embedding model. Supported options in d: - input (string): the text to embed (required) - truncate (bool): if true, truncate input to fit context window (default: false) - truncate_direction (string): "right" (default) or "left" - dimensions (int): reduce output to first N dimensions (for Matryoshka models) Each model instance processes calls sequentially (llama.cpp only supports sequence 0 for embedding extraction). Use NSeqMax > 1 to create multiple model instances for concurrent request handling. Batch multiple texts in the input parameter for better performance within a single request.'})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-embeddingshttp",children:[e.jsx("h4",{children:"Kronk.EmbeddingsHTTP"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) EmbeddingsHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.EmbedReponse, error)"})}),e.jsx("p",{className:"doc-description",children:"EmbeddingsHTTP provides http handler support for an embeddings call."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-modelconfig",children:[e.jsx("h4",{children:"Kronk.ModelConfig"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) ModelConfig() model.Config"})}),e.jsx("p",{className:"doc-description",children:"ModelConfig returns a copy of the configuration being used. This may be different from the configuration passed to New() if the model has overridden any of the settings."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-modelinfo",children:[e.jsx("h4",{children:"Kronk.ModelInfo"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) ModelInfo() model.ModelInfo"})}),e.jsx("p",{className:"doc-description",children:"ModelInfo returns the model information."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-rerank",children:[e.jsx("h4",{children:"Kronk.Rerank"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) Rerank(ctx context.Context, d model.D) (model.RerankResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"Rerank provides support to interact with a reranker model. Supported options in d: - query (string): the query to rank documents against (required) - documents ([]string): the documents to rank (required) - top_n (int): return only the top N results (optional, default: all) - return_documents (bool): include document text in results (default: false) Each model instance processes calls sequentially (llama.cpp only supports sequence 0 for rerank extraction). Use NSeqMax > 1 to create multiple model instances for concurrent request handling. Batch multiple texts in the input parameter for better performance within a single request."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-rerankhttp",children:[e.jsx("h4",{children:"Kronk.RerankHTTP"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) RerankHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.RerankResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"RerankHTTP provides http handler support for a rerank call."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-response",children:[e.jsx("h4",{children:"Kronk.Response"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) Response(ctx context.Context, d model.D) (ResponseResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"Response provides support to interact with an inference model. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-responsestreaming",children:[e.jsx("h4",{children:"Kronk.ResponseStreaming"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) ResponseStreaming(ctx context.Context, d model.D) (<-chan ResponseStreamEvent, error)"})}),e.jsx("p",{className:"doc-description",children:"ResponseStreaming provides streaming support for the Responses API. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-responsestreaminghttp",children:[e.jsx("h4",{children:"Kronk.ResponseStreamingHTTP"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) ResponseStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (ResponseResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"ResponseStreamingHTTP provides http handler support for a responses call. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-systeminfo",children:[e.jsx("h4",{children:"Kronk.SystemInfo"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) SystemInfo() map[string]string"})}),e.jsx("p",{className:"doc-description",children:"SystemInfo returns system information."})]}),e.jsxs("div",{className:"doc-section",id:"method-kronk-unload",children:[e.jsx("h4",{children:"Kronk.Unload"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (krn *Kronk) Unload(ctx context.Context) error"})}),e.jsx("p",{className:"doc-description",children:"Unload will close down the loaded model. You should call this only when you are completely done using Kronk."})]}),e.jsxs("div",{className:"doc-section",id:"method-loglevel-int",children:[e.jsx("h4",{children:"LogLevel.Int"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (ll LogLevel) Int() int"})}),e.jsx("p",{className:"doc-description",children:"Int returns the integer value."})]})]}),e.jsxs("div",{className:"card",id:"constants",children:[e.jsx("h3",{children:"Constants"}),e.jsxs("div",{className:"doc-section",id:"const-version",children:[e.jsx("h4",{children:"Version"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'const Version = "1.15.1"'})}),e.jsx("p",{className:"doc-description",children:"Version contains the current version of the kronk package."})]})]}),e.jsxs("div",{className:"card",id:"variables",children:[e.jsx("h3",{children:"Variables"}),e.jsxs("div",{className:"doc-section",id:"var-discardlogger",children:[e.jsx("h4",{children:"DiscardLogger"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`var DiscardLogger = func(ctx context.Context, msg string, args ...any) {
}`})}),e.jsx("p",{className:"doc-description",children:"DiscardLogger discards logging."})]}),e.jsxs("div",{className:"doc-section",id:"var-fmtlogger",children:[e.jsx("h4",{children:"FmtLogger"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`var FmtLogger = func(ctx context.Context, msg string, args ...any) {
	traceID, ok := ctx.Value(traceIDKey(1)).(string)
	switch ok {
	case true:
		fmt.Printf("traceID: %s: %s:", traceID, msg)
	default:
		fmt.Printf("%s:", msg)
	}

	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v[%v]", args[i], args[i+1])
		}
	}

	if len(msg) > 0 && msg[0] != '\\r' {
		fmt.Println()
	}
}`})}),e.jsx("p",{className:"doc-description",children:"FmtLogger provides a basic logger that writes to stdout."})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#functions",className:"doc-index-header",children:"Functions"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#func-init",children:"Init"})}),e.jsx("li",{children:e.jsx("a",{href:"#func-setfmtloggertraceid",children:"SetFmtLoggerTraceID"})}),e.jsx("li",{children:e.jsx("a",{href:"#func-new",children:"New"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#types",className:"doc-index-header",children:"Types"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#type-incompletedetail",children:"IncompleteDetail"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-initoption",children:"InitOption"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-inputtokensdetails",children:"InputTokensDetails"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-kronk",children:"Kronk"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-loglevel",children:"LogLevel"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-logger",children:"Logger"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-option",children:"Option"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-outputtokensdetails",children:"OutputTokensDetails"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responsecontentitem",children:"ResponseContentItem"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responseerror",children:"ResponseError"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responseformattype",children:"ResponseFormatType"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responseoutputitem",children:"ResponseOutputItem"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responsereasoning",children:"ResponseReasoning"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responseresponse",children:"ResponseResponse"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responsestreamevent",children:"ResponseStreamEvent"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responsetextformat",children:"ResponseTextFormat"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responseusage",children:"ResponseUsage"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#methods",className:"doc-index-header",children:"Methods"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-activestreams",children:"Kronk.ActiveStreams"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-chat",children:"Kronk.Chat"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-chatstreaming",children:"Kronk.ChatStreaming"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-chatstreaminghttp",children:"Kronk.ChatStreamingHTTP"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-embeddings",children:"Kronk.Embeddings"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-embeddingshttp",children:"Kronk.EmbeddingsHTTP"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-modelconfig",children:"Kronk.ModelConfig"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-modelinfo",children:"Kronk.ModelInfo"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-rerank",children:"Kronk.Rerank"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-rerankhttp",children:"Kronk.RerankHTTP"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-response",children:"Kronk.Response"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-responsestreaming",children:"Kronk.ResponseStreaming"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-responsestreaminghttp",children:"Kronk.ResponseStreamingHTTP"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-systeminfo",children:"Kronk.SystemInfo"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-kronk-unload",children:"Kronk.Unload"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-loglevel-int",children:"LogLevel.Int"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#constants",className:"doc-index-header",children:"Constants"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#const-version",children:"Version"})})})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#variables",className:"doc-index-header",children:"Variables"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#var-discardlogger",children:"DiscardLogger"})}),e.jsx("li",{children:e.jsx("a",{href:"#var-fmtlogger",children:"FmtLogger"})})]})]})]})})]})]})}function yf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Model Package"}),e.jsx("p",{children:"Package model provides the low-level api for working with models."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",children:[e.jsx("h3",{children:"Import"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'import "github.com/ardanlabs/kronk/sdk/kronk/model"'})})]}),e.jsxs("div",{className:"card",id:"functions",children:[e.jsx("h3",{children:"Functions"}),e.jsxs("div",{className:"doc-section",id:"func-checkmodel",children:[e.jsx("h4",{children:"CheckModel"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func CheckModel(modelFile string, checkSHA bool) error"})}),e.jsx("p",{className:"doc-description",children:"CheckModel is check if the downloaded model is valid based on it's sha file. If no sha file exists, this check will return with no error."})]}),e.jsxs("div",{className:"doc-section",id:"func-parseggmltype",children:[e.jsx("h4",{children:"ParseGGMLType"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func ParseGGMLType(s string) (GGMLType, error)"})}),e.jsx("p",{className:"doc-description",children:'ParseGGMLType parses a string into a GGMLType. Supported values: "f32", "f16", "q4_0", "q4_1", "q5_0", "q5_1", "q8_0", "bf16", "auto".'})]}),e.jsxs("div",{className:"doc-section",id:"func-newmodel",children:[e.jsx("h4",{children:"NewModel"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func NewModel(ctx context.Context, tmplRetriever TemplateRetriever, cfg Config) (*Model, error)"})})]}),e.jsxs("div",{className:"doc-section",id:"func-parsesplitmode",children:[e.jsx("h4",{children:"ParseSplitMode"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func ParseSplitMode(s string) (SplitMode, error)"})}),e.jsx("p",{className:"doc-description",children:'ParseSplitMode parses a string into a SplitMode. Supported values: "none", "layer", "row", "expert-parallel", "tensor-parallel".'})]})]}),e.jsxs("div",{className:"card",id:"types",children:[e.jsx("h3",{children:"Types"}),e.jsxs("div",{className:"doc-section",id:"type-chatresponse",children:[e.jsx("h4",{children:"ChatResponse"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ChatResponse struct {\n	ID      string   `json:"id"`\n	Object  string   `json:"object"`\n	Created int64    `json:"created"`\n	Model   string   `json:"model"`\n	Choice  []Choice `json:"choices"`\n	Usage   *Usage   `json:"usage,omitempty"`\n	Prompt  string   `json:"prompt,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ChatResponse represents output for inference models."})]}),e.jsxs("div",{className:"doc-section",id:"type-choice",children:[e.jsx("h4",{children:"Choice"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type Choice struct {\n	Index           int              `json:"index"`\n	Message         *ResponseMessage `json:"message,omitempty"`\n	Delta           *ResponseMessage `json:"delta,omitempty"`\n	Logprobs        *Logprobs        `json:"logprobs,omitempty"`\n	FinishReasonPtr *string          `json:"finish_reason"`\n}'})}),e.jsx("p",{className:"doc-description",children:"Choice represents a single choice in a response."})]}),e.jsxs("div",{className:"doc-section",id:"type-config",children:[e.jsx("h4",{children:"Config"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`type Config struct {
	Log                  Logger
	ModelFiles           []string
	ProjFile             string
	JinjaFile            string
	Device               string
	ContextWindow        int
	NBatch               int
	NUBatch              int
	NThreads             int
	NThreadsBatch        int
	CacheTypeK           GGMLType
	CacheTypeV           GGMLType
	FlashAttention       FlashAttentionType
	UseDirectIO          bool
	IgnoreIntegrityCheck bool
	NSeqMax              int
	OffloadKQV           *bool
	OpOffload            *bool
	NGpuLayers           *int32
	SplitMode            SplitMode
	SystemPromptCache    bool
	FirstMessageCache    bool
	CacheMinTokens       int
}`})}),e.jsx("p",{className:"doc-description",children:`Config represents model level configuration. These values if configured incorrectly can cause the system to panic. The defaults are used when these values are set to 0. ModelInstances is the number of instances of the model to create. Unless you have more than 1 GPU, the recommended number of instances is 1. ModelFiles is the path to the model files. This is mandatory to provide. ProjFiles is the path to the projection files. This is mandatory for media based models like vision and audio. JinjaFile is the path to the jinja file. This is not required and can be used if you want to override the templated provided by the model metadata. Device is the device to use for the model. If not set, the default device will be used. To see what devices are available, run the following command which will be found where you installed llama.cpp. $ llama-bench --list-devices ContextWindow (often referred to as context length) is the maximum number of tokens that a large language model can process and consider at one time when generating a response. It defines the model's effective "memory" for a single conversation or text generation task. When set to 0, the default value is 4096. NBatch is the logical batch size or the maximum number of tokens that can be in a single forward pass through the model at any given time. It defines the maximum capacity of the processing batch. If you are processing a very long prompt or multiple prompts simultaneously, the total number of tokens processed in one go will not exceed NBatch. Increasing n_batch can improve performance (throughput) if your hardware can handle it, as it better utilizes parallel computation. However, a very high n_batch can lead to out-of-memory errors on systems with limited VRAM. When set to 0, the default value is 2048. NUBatch is the physical batch size or the maximum number of tokens processed together during the initial prompt processing phase (also called "prompt ingestion") to populate the KV cache. It specifically optimizes the initial loading of prompt tokens into the KV cache. If a prompt is longer than NUBatch, it will be broken down and processed in chunks of n_ubatch tokens sequentially. This parameter is crucial for tuning performance on specific hardware (especially GPUs) because different values might yield better prompt processing times depending on the memory architecture. When set to 0, the default value is 512. NThreads is the number of threads to use for generation. When set to 0, the default llama.cpp value is used. NThreadsBatch is the number of threads to use for batch processing. When set to 0, the default llama.cpp value is used. CacheTypeK is the data type for the K (key) cache. This controls the precision of the key vectors in the KV cache. Lower precision types (like Q8_0 or Q4_0) reduce memory usage but may slightly affect quality. When set to GGMLTypeAuto or left as zero value, the default llama.cpp value (F16) is used. CacheTypeV is the data type for the V (value) cache. This controls the precision of the value vectors in the KV cache. When set to GGMLTypeAuto or left as zero value, the default llama.cpp value (F16) is used. FlashAttention controls Flash Attention mode. Flash Attention reduces memory usage and speeds up attention computation, especially for large context windows. When left as zero value, FlashAttentionEnabled is used (default on). Set to FlashAttentionDisabled to disable, or FlashAttentionAuto to let llama.cpp decide. IgnoreIntegrityCheck is a boolean that determines if the system should ignore a model integrity check before trying to use it. NSeqMax controls concurrency behavior based on model type. For text inference models, it sets the maximum number of sequences processed in parallel within a single model instance (batched inference). For sequential models (embeddings, reranking, vision, audio), it creates that many model instances in a pool for concurrent request handling. When set to 0, a default of 1 is used. OffloadKQV controls whether the KV cache is offloaded to the GPU. When nil or true, the KV cache is stored on the GPU (default behavior). Set to false to keep the KV cache on the CPU, which reduces VRAM usage but may slow inference. OpOffload controls whether host tensor operations are offloaded to the device (GPU). When nil or true, operations are offloaded (default behavior). Set to false to keep operations on the CPU. NGpuLayers is the number of model layers to offload to the GPU. When set to 0, all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any positive value specifies the exact number of layers to offload. SplitMode controls how the model is split across multiple GPUs: - SplitModeNone (0): single GPU - SplitModeLayer (1): split layers and KV across GPUs - SplitModeRow (2): split layers and KV across GPUs with tensor parallelism (recommended for MoE models like Qwen3-MoE, Mixtral, DeepSeek) When not set, defaults to SplitModeRow for optimal MoE performance. SystemPromptCache enables caching of system prompt KV state. When enabled, the first message with role="system" is cached. The system prompt is evaluated once and its KV cache is copied to all client sequences on subsequent requests with the same system prompt. This avoids redundant prefill computation for applications that use a consistent system prompt. The cache is automatically invalidated and re-evaluated when the system prompt changes. FirstMessageCache enables caching of the first user message's KV state. This supports clients like Cline that use a large first user message as context. The first message with role="user" is cached. The cache is invalidated when the first user message changes. Both SystemPromptCache and FirstMessageCache can be enabled simultaneously. When both are enabled, they use separate sequences (seq 0 for SPC, seq 1 for FMC) and the memory overhead is +2 context windows. CacheMinTokens sets the minimum token count required before caching. Messages shorter than this threshold are not cached, as the overhead of cache management may outweigh the prefill savings. When set to 0, defaults to 100 tokens.`})]}),e.jsxs("div",{className:"doc-section",id:"type-contentlogprob",children:[e.jsx("h4",{children:"ContentLogprob"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ContentLogprob struct {\n	Token       string       `json:"token"`\n	Logprob     float32      `json:"logprob"`\n	Bytes       []byte       `json:"bytes,omitempty"`\n	TopLogprobs []TopLogprob `json:"top_logprobs,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ContentLogprob represents log probability information for a single token."})]}),e.jsxs("div",{className:"doc-section",id:"type-d",children:[e.jsx("h4",{children:"D"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type D map[string]any"})}),e.jsx("p",{className:"doc-description",children:"D represents a generic docment of fields and values."})]}),e.jsxs("div",{className:"doc-section",id:"type-embeddata",children:[e.jsx("h4",{children:"EmbedData"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type EmbedData struct {\n	Object    string    `json:"object"`\n	Index     int       `json:"index"`\n	Embedding []float32 `json:"embedding"`\n}'})}),e.jsx("p",{className:"doc-description",children:"EmbedData represents the data associated with an embedding call."})]}),e.jsxs("div",{className:"doc-section",id:"type-embedreponse",children:[e.jsx("h4",{children:"EmbedReponse"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type EmbedReponse struct {\n	Object  string      `json:"object"`\n	Created int64       `json:"created"`\n	Model   string      `json:"model"`\n	Data    []EmbedData `json:"data"`\n	Usage   EmbedUsage  `json:"usage"`\n}'})}),e.jsx("p",{className:"doc-description",children:"EmbedReponse represents the output for an embedding call."})]}),e.jsxs("div",{className:"doc-section",id:"type-embedusage",children:[e.jsx("h4",{children:"EmbedUsage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type EmbedUsage struct {\n	PromptTokens int `json:"prompt_tokens"`\n	TotalTokens  int `json:"total_tokens"`\n}'})}),e.jsx("p",{className:"doc-description",children:"EmbedUsage provides token usage information for embeddings."})]}),e.jsxs("div",{className:"doc-section",id:"type-flashattentiontype",children:[e.jsx("h4",{children:"FlashAttentionType"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type FlashAttentionType int32"})}),e.jsx("p",{className:"doc-description",children:"FlashAttentionType controls when to enable Flash Attention. Flash Attention reduces memory usage and speeds up attention computation, especially beneficial for large context windows."})]}),e.jsxs("div",{className:"doc-section",id:"type-ggmltype",children:[e.jsx("h4",{children:"GGMLType"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type GGMLType int32"})}),e.jsx("p",{className:"doc-description",children:"GGMLType represents a ggml data type for the KV cache. These values correspond to the ggml_type enum in llama.cpp."})]}),e.jsxs("div",{className:"doc-section",id:"type-logger",children:[e.jsx("h4",{children:"Logger"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type Logger func(ctx context.Context, msg string, args ...any)"})}),e.jsx("p",{className:"doc-description",children:"Logger provides a function for logging messages from different APIs."})]}),e.jsxs("div",{className:"doc-section",id:"type-logprobs",children:[e.jsx("h4",{children:"Logprobs"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type Logprobs struct {\n	Content []ContentLogprob `json:"content,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:"Logprobs contains log probability information for the response."})]}),e.jsxs("div",{className:"doc-section",id:"type-mediatype",children:[e.jsx("h4",{children:"MediaType"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type MediaType int"})})]}),e.jsxs("div",{className:"doc-section",id:"type-model",children:[e.jsx("h4",{children:"Model"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`type Model struct {
	// Has unexported fields.
}`})}),e.jsx("p",{className:"doc-description",children:"Model represents a model and provides a low-level API for working with it."})]}),e.jsxs("div",{className:"doc-section",id:"type-modelinfo",children:[e.jsx("h4",{children:"ModelInfo"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`type ModelInfo struct {
	ID            string
	HasProjection bool
	Desc          string
	Size          uint64
	HasEncoder    bool
	HasDecoder    bool
	IsRecurrent   bool
	IsHybrid      bool
	IsGPTModel    bool
	IsEmbedModel  bool
	IsRerankModel bool
	Metadata      map[string]string
	TemplateFile  string
	Template      Template
}`})}),e.jsx("p",{className:"doc-description",children:"ModelInfo represents the model's card information."})]}),e.jsxs("div",{className:"doc-section",id:"type-rerankresponse",children:[e.jsx("h4",{children:"RerankResponse"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type RerankResponse struct {\n	Object  string         `json:"object"`\n	Created int64          `json:"created"`\n	Model   string         `json:"model"`\n	Data    []RerankResult `json:"data"`\n	Usage   RerankUsage    `json:"usage"`\n}'})}),e.jsx("p",{className:"doc-description",children:"RerankResponse represents the output for a reranking call."})]}),e.jsxs("div",{className:"doc-section",id:"type-rerankresult",children:[e.jsx("h4",{children:"RerankResult"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type RerankResult struct {\n	Index          int     `json:"index"`\n	RelevanceScore float32 `json:"relevance_score"`\n	Document       string  `json:"document,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:"RerankResult represents a single document's reranking result."})]}),e.jsxs("div",{className:"doc-section",id:"type-rerankusage",children:[e.jsx("h4",{children:"RerankUsage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type RerankUsage struct {\n	PromptTokens int `json:"prompt_tokens"`\n	TotalTokens  int `json:"total_tokens"`\n}'})}),e.jsx("p",{className:"doc-description",children:"RerankUsage provides token usage information for reranking."})]}),e.jsxs("div",{className:"doc-section",id:"type-responsemessage",children:[e.jsx("h4",{children:"ResponseMessage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseMessage struct {\n	Role      string             `json:"role,omitempty"`\n	Content   string             `json:"content,omitempty"`\n	Reasoning string             `json:"reasoning_content,omitempty"`\n	ToolCalls []ResponseToolCall `json:"tool_calls,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:"ResponseMessage represents a single message in a response."})]}),e.jsxs("div",{className:"doc-section",id:"type-responsetoolcall",children:[e.jsx("h4",{children:"ResponseToolCall"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseToolCall struct {\n	ID       string                   `json:"id"`\n	Index    int                      `json:"index"`\n	Type     string                   `json:"type"`\n	Function ResponseToolCallFunction `json:"function"`\n	Status   int                      `json:"status,omitempty"`\n	Raw      string                   `json:"raw,omitempty"`\n	Error    string                   `json:"error,omitempty"`\n}'})})]}),e.jsxs("div",{className:"doc-section",id:"type-responsetoolcallfunction",children:[e.jsx("h4",{children:"ResponseToolCallFunction"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type ResponseToolCallFunction struct {\n	Name      string            `json:"name"`\n	Arguments ToolCallArguments `json:"arguments"`\n}'})})]}),e.jsxs("div",{className:"doc-section",id:"type-splitmode",children:[e.jsx("h4",{children:"SplitMode"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type SplitMode int32"})}),e.jsx("p",{className:"doc-description",children:"SplitMode controls how the model is split across multiple GPUs. This is particularly important for Mixture of Experts (MoE) models."})]}),e.jsxs("div",{className:"doc-section",id:"type-template",children:[e.jsx("h4",{children:"Template"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`type Template struct {
	FileName string
	Script   string
}`})}),e.jsx("p",{className:"doc-description",children:"Template provides the template file name."})]}),e.jsxs("div",{className:"doc-section",id:"type-templateretriever",children:[e.jsx("h4",{children:"TemplateRetriever"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`type TemplateRetriever interface {
	Retrieve(modelID string) (Template, error)
}`})}),e.jsx("p",{className:"doc-description",children:"TemplateRetriever returns a configured template for a model."})]}),e.jsxs("div",{className:"doc-section",id:"type-toolcallarguments",children:[e.jsx("h4",{children:"ToolCallArguments"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"type ToolCallArguments map[string]any"})}),e.jsx("p",{className:"doc-description",children:"ToolCallArguments represents tool call arguments that marshal to a JSON string per OpenAI API spec, but can unmarshal from either a string or object."})]}),e.jsxs("div",{className:"doc-section",id:"type-toplogprob",children:[e.jsx("h4",{children:"TopLogprob"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type TopLogprob struct {\n	Token   string  `json:"token"`\n	Logprob float32 `json:"logprob"`\n	Bytes   []byte  `json:"bytes,omitempty"`\n}'})}),e.jsx("p",{className:"doc-description",children:"TopLogprob represents a single token with its log probability."})]}),e.jsxs("div",{className:"doc-section",id:"type-usage",children:[e.jsx("h4",{children:"Usage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:'type Usage struct {\n	PromptTokens     int     `json:"prompt_tokens"`\n	ReasoningTokens  int     `json:"reasoning_tokens"`\n	CompletionTokens int     `json:"completion_tokens"`\n	OutputTokens     int     `json:"output_tokens"`\n	TotalTokens      int     `json:"total_tokens"`\n	TokensPerSecond  float64 `json:"tokens_per_second"`\n}'})}),e.jsx("p",{className:"doc-description",children:"Usage provides details usage information for the request."})]})]}),e.jsxs("div",{className:"card",id:"methods",children:[e.jsx("h3",{children:"Methods"}),e.jsxs("div",{className:"doc-section",id:"method-choice-finishreason",children:[e.jsx("h4",{children:"Choice.FinishReason"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (c Choice) FinishReason() string"})}),e.jsx("p",{className:"doc-description",children:"FinishReason return the finish reason as an empty string if it is nil."})]}),e.jsxs("div",{className:"doc-section",id:"method-d-clone",children:[e.jsx("h4",{children:"D.Clone"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (d D) Clone() D"})}),e.jsx("p",{className:"doc-description",children:"Clone creates a shallow copy of the document. This is useful when you need to modify the document without affecting the original."})]}),e.jsxs("div",{className:"doc-section",id:"method-d-logsafe",children:[e.jsx("h4",{children:"D.LogSafe"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (d D) LogSafe() D"})}),e.jsx("p",{className:"doc-description",children:"LogSafe returns a copy of the document containing only fields that are safe to log. This excludes sensitive fields like messages and input which may contain private user data."})]}),e.jsxs("div",{className:"doc-section",id:"method-flashattentiontype-unmarshalyaml",children:[e.jsx("h4",{children:"FlashAttentionType.UnmarshalYAML"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (t *FlashAttentionType) UnmarshalYAML(unmarshal func(interface{}) error) error"})}),e.jsx("p",{className:"doc-description",children:"UnmarshalYAML implements yaml.Unmarshaler to parse string values."})]}),e.jsxs("div",{className:"doc-section",id:"method-ggmltype-string",children:[e.jsx("h4",{children:"GGMLType.String"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (t GGMLType) String() string"})}),e.jsx("p",{className:"doc-description",children:"String returns the string representation of a GGMLType."})]}),e.jsxs("div",{className:"doc-section",id:"method-ggmltype-toyzmatype",children:[e.jsx("h4",{children:"GGMLType.ToYZMAType"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (t GGMLType) ToYZMAType() llama.GGMLType"})})]}),e.jsxs("div",{className:"doc-section",id:"method-ggmltype-unmarshalyaml",children:[e.jsx("h4",{children:"GGMLType.UnmarshalYAML"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (t *GGMLType) UnmarshalYAML(unmarshal func(interface{}) error) error"})}),e.jsx("p",{className:"doc-description",children:'UnmarshalYAML implements yaml.Unmarshaler to parse string values like "f16".'})]}),e.jsxs("div",{className:"doc-section",id:"method-model-chat",children:[e.jsx("h4",{children:"Model.Chat"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"Chat performs a chat request and returns the final response. Text inference requests can run concurrently based on the NSeqMax config value, which controls parallel sequence processing. However, requests that include vision or audio content are processed sequentially due to media pipeline constraints."})]}),e.jsxs("div",{className:"doc-section",id:"method-model-chatstreaming",children:[e.jsx("h4",{children:"Model.ChatStreaming"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (m *Model) ChatStreaming(ctx context.Context, d D) <-chan ChatResponse"})}),e.jsx("p",{className:"doc-description",children:"ChatStreaming performs a chat request and streams the response. Text inference requests can run concurrently based on the NSeqMax config value, which controls parallel sequence processing. However, requests that include vision or audio content are processed sequentially due to media pipeline constraints."})]}),e.jsxs("div",{className:"doc-section",id:"method-model-config",children:[e.jsx("h4",{children:"Model.Config"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (m *Model) Config() Config"})})]}),e.jsxs("div",{className:"doc-section",id:"method-model-embeddings",children:[e.jsx("h4",{children:"Model.Embeddings"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (m *Model) Embeddings(ctx context.Context, d D) (EmbedReponse, error)"})}),e.jsx("p",{className:"doc-description",children:'Embeddings performs batch embedding for multiple inputs in a single forward pass. This is more efficient than calling Embeddings multiple times. Supported options in d: - input ([]string): the texts to embed (required) - truncate (bool): if true, truncate inputs to fit context window (default: false) - truncate_direction (string): "right" (default) or "left" - dimensions (int): reduce output to first N dimensions (for Matryoshka models) Each model instance processes calls sequentially (llama.cpp only supports sequence 0 for embedding extraction). Use NSeqMax > 1 to create multiple model instances for concurrent request handling. Batch multiple texts in the input parameter for better performance within a single request.'})]}),e.jsxs("div",{className:"doc-section",id:"method-model-modelinfo",children:[e.jsx("h4",{children:"Model.ModelInfo"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (m *Model) ModelInfo() ModelInfo"})})]}),e.jsxs("div",{className:"doc-section",id:"method-model-rerank",children:[e.jsx("h4",{children:"Model.Rerank"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (m *Model) Rerank(ctx context.Context, d D) (RerankResponse, error)"})}),e.jsx("p",{className:"doc-description",children:"Rerank performs reranking for a query against multiple documents. It scores each document's relevance to the query and returns results sorted by relevance score (highest first). Supported options in d: - query (string): the query to rank documents against (required) - documents ([]string): the documents to rank (required) - top_n (int): return only the top N results (optional, default: all) - return_documents (bool): include document text in results (default: false) Each model instance processes calls sequentially (llama.cpp only supports sequence 0 for rerank extraction). Use NSeqMax > 1 to create multiple model instances for concurrent request handling. Batch multiple texts in the input parameter for better performance within a single request."})]}),e.jsxs("div",{className:"doc-section",id:"method-model-unload",children:[e.jsx("h4",{children:"Model.Unload"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (m *Model) Unload(ctx context.Context) error"})})]}),e.jsxs("div",{className:"doc-section",id:"method-splitmode-string",children:[e.jsx("h4",{children:"SplitMode.String"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (s SplitMode) String() string"})}),e.jsx("p",{className:"doc-description",children:"String returns the string representation of a SplitMode."})]}),e.jsxs("div",{className:"doc-section",id:"method-splitmode-toyzmatype",children:[e.jsx("h4",{children:"SplitMode.ToYZMAType"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (s SplitMode) ToYZMAType() llama.SplitMode"})}),e.jsx("p",{className:"doc-description",children:"ToYZMAType converts to the yzma/llama.cpp SplitMode type."})]}),e.jsxs("div",{className:"doc-section",id:"method-splitmode-unmarshalyaml",children:[e.jsx("h4",{children:"SplitMode.UnmarshalYAML"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (s *SplitMode) UnmarshalYAML(unmarshal func(interface{}) error) error"})}),e.jsx("p",{className:"doc-description",children:"UnmarshalYAML implements yaml.Unmarshaler to parse string values."})]}),e.jsxs("div",{className:"doc-section",id:"method-toolcallarguments-marshaljson",children:[e.jsx("h4",{children:"ToolCallArguments.MarshalJSON"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (a ToolCallArguments) MarshalJSON() ([]byte, error)"})})]}),e.jsxs("div",{className:"doc-section",id:"method-toolcallarguments-unmarshaljson",children:[e.jsx("h4",{children:"ToolCallArguments.UnmarshalJSON"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"func (a *ToolCallArguments) UnmarshalJSON(data []byte) error"})})]})]}),e.jsxs("div",{className:"card",id:"constants",children:[e.jsx("h3",{children:"Constants"}),e.jsxs("div",{className:"doc-section",id:"const-objectchatunknown",children:[e.jsx("h4",{children:"ObjectChatUnknown"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`const (
	ObjectChatUnknown   = "chat.unknown"
	ObjectChatText      = "chat.completion.chunk"
	ObjectChatTextFinal = "chat.completion"
	ObjectChatMedia     = "chat.media"
)`})}),e.jsx("p",{className:"doc-description",children:"Objects represent the different types of data that is being processed."})]}),e.jsxs("div",{className:"doc-section",id:"const-roleuser",children:[e.jsx("h4",{children:"RoleUser"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)`})}),e.jsx("p",{className:"doc-description",children:"Roles represent the different roles that can be used in a chat."})]}),e.jsxs("div",{className:"doc-section",id:"const-finishreasonstop",children:[e.jsx("h4",{children:"FinishReasonStop"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`const (
	FinishReasonStop  = "stop"
	FinishReasonTool  = "tool_calls"
	FinishReasonError = "error"
)`})}),e.jsx("p",{className:"doc-description",children:"FinishReasons represent the different reasons a response can be finished."})]}),e.jsxs("div",{className:"doc-section",id:"const-thinkingenabled",children:[e.jsx("h4",{children:"ThinkingEnabled"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`const (
	// The model will perform thinking. This is the default setting.
	ThinkingEnabled = "true"

	// The model will not perform thinking.
	ThinkingDisabled = "false"
)`})})]}),e.jsxs("div",{className:"doc-section",id:"const-reasoningeffortnone",children:[e.jsx("h4",{children:"ReasoningEffortNone"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`const (
	// The model does not perform reasoning This setting is fastest and lowest
	// cost, ideal for latency-sensitive tasks that do not require complex logic,
	// such as simple translation or data reformatting.
	ReasoningEffortNone = "none"

	// GPT: A very low amount of internal reasoning, optimized for throughput
	// and speed.
	ReasoningEffortMinimal = "minimal"

	// GPT: Light reasoning that favors speed and lower token usage, suitable
	// for triage or short answers.
	ReasoningEffortLow = "low"

	// GPT: The default setting, providing a balance between speed and reasoning
	// accuracy. This is a good general-purpose choice for most tasks like
	// content drafting or standard Q&A.
	ReasoningEffortMedium = "medium"

	// GPT: Extensive reasoning for complex, multi-step problems. This setting
	// leads to the most thorough and accurate analysis but increases latency
	// and cost due to a larger number of internal reasoning tokens used.
	ReasoningEffortHigh = "high"
)`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#functions",className:"doc-index-header",children:"Functions"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#func-checkmodel",children:"CheckModel"})}),e.jsx("li",{children:e.jsx("a",{href:"#func-parseggmltype",children:"ParseGGMLType"})}),e.jsx("li",{children:e.jsx("a",{href:"#func-newmodel",children:"NewModel"})}),e.jsx("li",{children:e.jsx("a",{href:"#func-parsesplitmode",children:"ParseSplitMode"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#types",className:"doc-index-header",children:"Types"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#type-chatresponse",children:"ChatResponse"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-choice",children:"Choice"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-config",children:"Config"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-contentlogprob",children:"ContentLogprob"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-d",children:"D"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-embeddata",children:"EmbedData"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-embedreponse",children:"EmbedReponse"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-embedusage",children:"EmbedUsage"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-flashattentiontype",children:"FlashAttentionType"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-ggmltype",children:"GGMLType"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-logger",children:"Logger"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-logprobs",children:"Logprobs"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-mediatype",children:"MediaType"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-model",children:"Model"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-modelinfo",children:"ModelInfo"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-rerankresponse",children:"RerankResponse"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-rerankresult",children:"RerankResult"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-rerankusage",children:"RerankUsage"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responsemessage",children:"ResponseMessage"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responsetoolcall",children:"ResponseToolCall"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-responsetoolcallfunction",children:"ResponseToolCallFunction"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-splitmode",children:"SplitMode"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-template",children:"Template"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-templateretriever",children:"TemplateRetriever"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-toolcallarguments",children:"ToolCallArguments"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-toplogprob",children:"TopLogprob"})}),e.jsx("li",{children:e.jsx("a",{href:"#type-usage",children:"Usage"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#methods",className:"doc-index-header",children:"Methods"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#method-choice-finishreason",children:"Choice.FinishReason"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-d-clone",children:"D.Clone"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-d-logsafe",children:"D.LogSafe"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-flashattentiontype-unmarshalyaml",children:"FlashAttentionType.UnmarshalYAML"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-ggmltype-string",children:"GGMLType.String"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-ggmltype-toyzmatype",children:"GGMLType.ToYZMAType"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-ggmltype-unmarshalyaml",children:"GGMLType.UnmarshalYAML"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-model-chat",children:"Model.Chat"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-model-chatstreaming",children:"Model.ChatStreaming"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-model-config",children:"Model.Config"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-model-embeddings",children:"Model.Embeddings"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-model-modelinfo",children:"Model.ModelInfo"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-model-rerank",children:"Model.Rerank"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-model-unload",children:"Model.Unload"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-splitmode-string",children:"SplitMode.String"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-splitmode-toyzmatype",children:"SplitMode.ToYZMAType"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-splitmode-unmarshalyaml",children:"SplitMode.UnmarshalYAML"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-toolcallarguments-marshaljson",children:"ToolCallArguments.MarshalJSON"})}),e.jsx("li",{children:e.jsx("a",{href:"#method-toolcallarguments-unmarshaljson",children:"ToolCallArguments.UnmarshalJSON"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#constants",className:"doc-index-header",children:"Constants"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#const-objectchatunknown",children:"ObjectChatUnknown"})}),e.jsx("li",{children:e.jsx("a",{href:"#const-roleuser",children:"RoleUser"})}),e.jsx("li",{children:e.jsx("a",{href:"#const-finishreasonstop",children:"FinishReasonStop"})}),e.jsx("li",{children:e.jsx("a",{href:"#const-thinkingenabled",children:"ThinkingEnabled"})}),e.jsx("li",{children:e.jsx("a",{href:"#const-reasoningeffortnone",children:"ReasoningEffortNone"})})]})]})]})})]})]})}const kf=`// This example shows you a basic program of using Kronk to ask a model a question.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-question

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := question(krn); err != nil {
		fmt.Println(err)
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen3-8B-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
		CacheTypeK: model.GGMLTypeQ8_0,
		CacheTypeV: model.GGMLTypeQ8_0,
		NSeqMax:    2,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func question(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	question := "Hello model"

	fmt.Println()
	fmt.Println("QUESTION:", question)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, question),
		),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	// -------------------------------------------------------------------------

	var reasoning bool

	for resp := range ch {
		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			return nil

		default:
			if resp.Choice[0].Delta.Reasoning != "" {
				reasoning = true
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
				continue
			}

			if reasoning {
				reasoning = false
				fmt.Println()
				continue
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	return nil
}
`,Nf=`// This example shows you how to create a simple chat application against an
// inference model using kronk. Thanks to Kronk and yzma, reasoning and tool
// calling is enabled.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-chat

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

// const modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
// const modelURL = "https://huggingface.co/unsloth/GLM-4.7-Flash-GGUF/resolve/main/GLM-4.7-Flash-UD-Q8_K_XL.gguf"
const modelURL = "https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF/resolve/main/Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("run: unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("run: failed to unload model: %v", err)
		}
	}()

	if err := chat(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------
	// This is not mandatory if you won't be using models from the catalog. That
	// being said, if you are using a model that is part of the catalog with
	// a corrected jinja file, having the catalog system up to date will allow
	// the system to pull that jinja file.

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen3-8B-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles:        mp.ModelFiles,
		CacheTypeK:        model.GGMLTypeF16,
		CacheTypeV:        model.GGMLTypeF16,
		NSeqMax:           2,
		SystemPromptCache: true,
	}

	krn, err := kronk.New(cfg)

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

	return krn, nil
}

func chat(krn *kronk.Kronk) error {
	messages := model.DocumentArray()

	var systemPrompt = \`
		You are a helpful AI assistant. You are designed to help users answer
		questions, create content, and provide information in a helpful and
		accurate manner. Always follow the user's instructions carefully and
		respond with clear, concise, and well-structured answers. You are a
		helpful AI assistant. You are designed to help users answer questions,
		create content, and provide information in a helpful and accurate manner.
		Always follow the user's instructions carefully and respond with clear,
		concise, and well-structured answers. You are a helpful AI assistant.
		You are designed to help users answer questions, create content, and
		provide information in a helpful and accurate manner. Always follow the
		user's instructions carefully and respond with clear, concise, and
		well-structured answers.\`

	messages = append(messages,
		model.TextMessage(model.RoleSystem, systemPrompt),
	)

	for {
		var err error
		messages, err = userInput(messages)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("run:user input: %w", err)
		}

		messages, err = func() ([]model.D, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			d := model.D{
				"messages":       messages,
				"tools":          toolDocuments(),
				"max_tokens":     2048,
				"temperature":    0.7,
				"top_p":          0.8,
				"top_k":          20,
				"repeat_penalty": 1.05,
			}

			ch, err := performChat(ctx, krn, d)
			if err != nil {
				return nil, fmt.Errorf("run: unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krn, messages, ch)
			if err != nil {
				return nil, fmt.Errorf("run: model response: %w", err)
			}

			return messages, nil
		}()

		if err != nil {
			return fmt.Errorf("run: unable to perform chat: %w", err)
		}
	}
}

func userInput(messages []model.D) ([]model.D, error) {
	fmt.Print("\\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\\n')
	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	if userInput == "quit\\n" {
		return nil, io.EOF
	}

	messages = append(messages,
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, nil
}

func toolDocuments() []model.D {
	return model.DocumentArray(
		model.D{
			"type": "function",
			"function": model.D{
				"name":        "get_weather",
				"description": "Get the current weather for a location",
				"parameters": model.D{
					"type": "object",
					"properties": model.D{
						"location": model.D{
							"type":        "string",
							"description": "The location to get the weather for, e.g. San Francisco, CA",
						},
					},
					"required": []any{"location"},
				},
			},
		},
	)
}

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (<-chan model.ChatResponse, error) {
	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, ch <-chan model.ChatResponse) ([]model.D, error) {
	fmt.Print("\\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			messages = append(messages,
				model.TextMessage("assistant", resp.Choice[0].Delta.Content),
			)
			break loop

		case model.FinishReasonTool:
			fmt.Println()
			if krn.ModelInfo().IsGPTModel {
				fmt.Println()
			}

			fmt.Printf("\\u001b[92mModel Asking For Tool Calls:\\n\\u001b[0m")

			for _, tool := range resp.Choice[0].Delta.ToolCalls {
				fmt.Printf("\\u001b[92mToolID[%s]: %s(%s)\\n\\u001b[0m",
					tool.ID,
					tool.Function.Name,
					tool.Function.Arguments,
				)

				messages = append(messages,
					model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)\\n",
						tool.ID,
						tool.Function.Name,
						tool.Function.Arguments),
					),
				)
			}

			break loop

		default:
			if resp.Choice[0].Delta.Reasoning != "" {
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
				reasoning = true
				continue
			}

			if reasoning {
				reasoning = false

				fmt.Println()
				if krn.ModelInfo().IsGPTModel {
					fmt.Println()
				}
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
`,bf=`// This example shows you how to create a simple chat application against an
// inference model using the kronk Response api. Thanks to Kronk and yzma,
// reasoning and tool calling is enabled.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-response

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("run: unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("run: failed to unload model: %v", err)
		}
	}()

	if err := chat(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------
	// This is not mandatory if you won't be using models from the catalog. That
	// being said, if you are using a model that is part of the catalog with
	// a corrected jinja file, having the catalog system up to date will allow
	// the system to pull that jinja file.

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen3-8B-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
		CacheTypeK: model.GGMLTypeQ8_0,
		CacheTypeV: model.GGMLTypeQ8_0,
		NSeqMax:    2,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

	return krn, nil
}

func chat(krn *kronk.Kronk) error {
	messages := model.DocumentArray()

	for {
		var err error
		messages, err = userInput(messages)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("run:user input: %w", err)
		}

		messages, err = func() ([]model.D, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			d := model.D{
				"input":       messages,
				"tools":       toolDocuments(),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			}

			ch, err := performChat(ctx, krn, d)
			if err != nil {
				return nil, fmt.Errorf("run: unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krn, messages, ch)
			if err != nil {
				return nil, fmt.Errorf("run: model response: %w", err)
			}

			return messages, nil
		}()

		if err != nil {
			return fmt.Errorf("run: unable to perform chat: %w", err)
		}
	}
}

func userInput(messages []model.D) ([]model.D, error) {
	fmt.Print("\\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\\n')
	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	if userInput == "quit\\n" {
		return nil, io.EOF
	}

	messages = append(messages,
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, nil
}

func toolDocuments() []model.D {
	return model.DocumentArray(
		model.D{
			"type": "function",
			"function": model.D{
				"name":        "get_weather",
				"description": "Get the current weather for a location",
				"parameters": model.D{
					"type": "object",
					"properties": model.D{
						"location": model.D{
							"type":        "string",
							"description": "The location to get the weather for, e.g. San Francisco, CA",
						},
					},
					"required": []any{"location"},
				},
			},
		},
	)
}

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (<-chan kronk.ResponseStreamEvent, error) {
	ch, err := krn.ResponseStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("response streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, ch <-chan kronk.ResponseStreamEvent) ([]model.D, error) {
	fmt.Print("\\nMODEL> ")

	var fullText string
	var finalResp *kronk.ResponseResponse
	var reasoning bool

	for event := range ch {
		switch event.Type {
		case "response.reasoning_summary_text.delta":
			fmt.Printf("\\u001b[91m%s\\u001b[0m", event.Delta)
			reasoning = true

		case "response.output_text.delta":
			if reasoning {
				reasoning = false
				fmt.Println()
				if krn.ModelInfo().IsGPTModel {
					fmt.Println()
				}
			}
			fmt.Printf("%s", event.Delta)

		case "response.output_text.done":
			fullText = event.Text

		case "response.function_call_arguments.done":
			fmt.Println()
			if krn.ModelInfo().IsGPTModel {
				fmt.Println()
			}

			fmt.Printf("\\u001b[92mModel Asking For Tool Calls:\\n\\u001b[0m")
			fmt.Printf("\\u001b[92mToolID[%s]: %s(%s)\\n\\u001b[0m",
				event.ItemID,
				event.Name,
				event.Arguments,
			)

			messages = append(messages,
				model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%s)\\n",
					event.ItemID,
					event.Name,
					event.Arguments),
				),
			)

		case "response.completed":
			finalResp = event.Response
		}
	}

	if fullText != "" {
		messages = append(messages,
			model.TextMessage("assistant", fullText),
		)
	}

	// -------------------------------------------------------------------------

	if finalResp != nil {
		contextTokens := finalResp.Usage.InputTokens + finalResp.Usage.OutputTokens
		contextWindow := krn.ModelConfig().ContextWindow
		percentage := (float64(contextTokens) / float64(contextWindow)) * 100
		of := float32(contextWindow) / float32(1024)

		fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK)\\u001b[0m\\n",
			finalResp.Usage.InputTokens,
			finalResp.Usage.OutputTokenDetail.ReasoningTokens,
			finalResp.Usage.OutputTokens,
			finalResp.Usage.OutputTokens,
			contextTokens,
			percentage,
			of,
		)
	}

	return messages, nil
}
`,wf=`// This example shows you how to use an embedding model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-embedding

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const modelURL = "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := embedding(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("embeddinggemma-300m-qat-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:     mp.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func embedding(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	d := model.D{
		"input":              "Why is the sky blue?",
		"truncate":           true,
		"truncate_direction": "right",
	}

	resp, err := krn.Embeddings(ctx, d)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Model  :", resp.Model)
	fmt.Println("Object :", resp.Object)
	fmt.Println("Created:", time.UnixMilli(resp.Created))
	fmt.Println("  Index    :", resp.Data[0].Index)
	fmt.Println("  Object   :", resp.Data[0].Object)
	fmt.Printf("  Embedding: [%v...%v]\\n", resp.Data[0].Embedding[:3], resp.Data[0].Embedding[len(resp.Data[0].Embedding)-3:])

	return nil
}
`,Ef=`// This example shows you how to use a reranker model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-rerank

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const modelURL = "https://huggingface.co/gpustack/bge-reranker-v2-m3-GGUF/resolve/main/bge-reranker-v2-m3-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := rerank(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:    mp.ModelFiles,
		ContextWindow: 2048,
		NBatch:        2048,
		NUBatch:       512,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create reranker model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("  - reranking    :", krn.ModelInfo().IsRerankModel)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func rerank(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	d := model.D{
		"query": "What is the capital of France?",
		"documents": []string{
			"Paris is the capital and largest city of France.",
			"Berlin is the capital of Germany.",
			"The Eiffel Tower is located in Paris.",
			"London is the capital of England.",
			"France is a country in Western Europe.",
		},
		"top_n":            3,
		"return_documents": true,
	}

	resp, err := krn.Rerank(ctx, d)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Model  :", resp.Model)
	fmt.Println("Object :", resp.Object)
	fmt.Println("Created:", time.UnixMilli(resp.Created))
	fmt.Println()
	fmt.Println("Results (sorted by relevance):")
	for i, result := range resp.Data {
		fmt.Printf("  %d. Score: %.4f, Index: %d, Doc: %s\\n",
			i+1, result.RelevanceScore, result.Index, result.Document)
	}
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  Prompt Tokens:", resp.Usage.PromptTokens)
	fmt.Println("  Total Tokens :", resp.Usage.TotalTokens)

	return nil
}
`,Sf=`// This example shows you how to execute a simple prompt against a vision model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-vision

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const (
	modelURL  = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projURL   = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	imageFile = "examples/samples/giraffe.jpg"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	info, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(info)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := vision(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, projURL)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen2.5-VL-3B-Instruct-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:    mp.ModelFiles,
		ProjFile:      mp.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

	return krn, nil
}

func vision(krn *kronk.Kronk) error {
	question := "What is in this picture?"

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ch, err := performChat(ctx, krn, question, imageFile)
	if err != nil {
		return fmt.Errorf("perform chat: %w", err)
	}

	if err := modelResponse(krn, ch); err != nil {
		return fmt.Errorf("model response: %w", err)
	}

	return nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, question string, imageFile string) (<-chan model.ChatResponse, error) {
	image, err := readImage(imageFile)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	fmt.Printf("\\nQuestion: %s\\n", question)

	d := model.D{
		"messages":    model.RawMediaMessage(question, image),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("vision streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, ch <-chan model.ChatResponse) error {
	fmt.Print("\\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonStop:
			break loop

		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)
		}

		if resp.Choice[0].Delta.Reasoning != "" {
			fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
			reasoning = true
			continue
		}

		if reasoning {
			reasoning = false
			fmt.Print("\\n\\n")
		}

		fmt.Printf("%s", resp.Choice[0].Delta.Content)
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return nil
}

func readImage(imageFile string) ([]byte, error) {
	if _, err := os.Stat(imageFile); err != nil {
		return nil, fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	image, err := os.ReadFile(imageFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", imageFile, err)
	}

	return image, nil
}
`,Tf=`// This example shows you how to execute a simple prompt against a vision model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-vision

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const (
	modelURL  = "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf"
	projURL   = "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.mmproj-Q8_0.gguf"
	audioFile = "examples/samples/jfk.wav"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := audio(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, projURL)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen2-Audio-7B.Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:    mp.ModelFiles,
		ProjFile:      mp.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func audio(krn *kronk.Kronk) error {
	question := "Please describe what you hear in the following audio clip."

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ch, err := performChat(ctx, krn, question, audioFile)
	if err != nil {
		return fmt.Errorf("perform chat: %w", err)
	}

	if err := modelResponse(krn, ch); err != nil {
		return fmt.Errorf("model response: %w", err)
	}

	return nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, question string, imageFile string) (<-chan model.ChatResponse, error) {
	image, err := readImage(imageFile)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	fmt.Printf("\\nQuestion: %s\\n", question)

	d := model.D{
		"messages":    model.RawMediaMessage(question, image),
		"max_tokens":  2048,
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, ch <-chan model.ChatResponse) error {
	fmt.Print("\\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonStop:
			break loop

		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)
		}

		if resp.Choice[0].Delta.Reasoning != "" {
			fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
			reasoning = true
			continue
		}

		if reasoning {
			reasoning = false
			fmt.Print("\\n\\n")
		}

		fmt.Printf("%s", resp.Choice[0].Delta.Content)
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return nil
}

func readImage(imageFile string) ([]byte, error) {
	if _, err := os.Stat(imageFile); err != nil {
		return nil, fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	image, err := os.ReadFile(imageFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", imageFile, err)
	}

	return image, nil
}
`;function Rf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"SDK Examples"}),e.jsx("p",{children:"Complete working examples demonstrating how to use the Kronk SDK"})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"example-question",children:[e.jsx("h3",{children:"Question"}),e.jsx("p",{className:"doc-description",children:"Ask a single question to a model"}),e.jsx(hn,{code:kf,language:"go"})]}),e.jsxs("div",{className:"card",id:"example-chat",children:[e.jsx("h3",{children:"Chat"}),e.jsx("p",{className:"doc-description",children:"Interactive chat with conversation history"}),e.jsx(hn,{code:Nf,language:"go"})]}),e.jsxs("div",{className:"card",id:"example-response",children:[e.jsx("h3",{children:"Response"}),e.jsx("p",{className:"doc-description",children:"Interactive chat using the Response API with tool calling"}),e.jsx(hn,{code:bf,language:"go"})]}),e.jsxs("div",{className:"card",id:"example-embedding",children:[e.jsx("h3",{children:"Embedding"}),e.jsx("p",{className:"doc-description",children:"Generate embeddings for semantic search"}),e.jsx(hn,{code:wf,language:"go"})]}),e.jsxs("div",{className:"card",id:"example-rerank",children:[e.jsx("h3",{children:"Rerank"}),e.jsx("p",{className:"doc-description",children:"Rerank documents by relevance to a query"}),e.jsx(hn,{code:Ef,language:"go"})]}),e.jsxs("div",{className:"card",id:"example-vision",children:[e.jsx("h3",{children:"Vision"}),e.jsx("p",{className:"doc-description",children:"Analyze images using vision models"}),e.jsx(hn,{code:Sf,language:"go"})]}),e.jsxs("div",{className:"card",id:"example-audio",children:[e.jsx("h3",{children:"Audio"}),e.jsx("p",{className:"doc-description",children:"Process audio files using audio models"}),e.jsx(hn,{code:Tf,language:"go"})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsx("div",{className:"doc-sidebar-content",children:e.jsxs("div",{className:"doc-index-section",children:[e.jsx("span",{className:"doc-index-header",children:"Examples"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#example-question",children:"Question"})}),e.jsx("li",{children:e.jsx("a",{href:"#example-chat",children:"Chat"})}),e.jsx("li",{children:e.jsx("a",{href:"#example-response",children:"Response"})}),e.jsx("li",{children:e.jsx("a",{href:"#example-embedding",children:"Embedding"})}),e.jsx("li",{children:e.jsx("a",{href:"#example-rerank",children:"Rerank"})}),e.jsx("li",{children:e.jsx("a",{href:"#example-vision",children:"Vision"})}),e.jsx("li",{children:e.jsx("a",{href:"#example-audio",children:"Audio"})})]})]})})})]})]})}function Cf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"catalog"}),e.jsx("p",{children:"Manage model catalog - list and update available models."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"usage",children:[e.jsx("h3",{children:"Usage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk catalog <command> [flags]"})})]}),e.jsxs("div",{className:"card",id:"subcommands",children:[e.jsx("h3",{children:"Subcommands"}),e.jsxs("div",{className:"doc-section",id:"cmd-list",children:[e.jsx("h4",{children:"list"}),e.jsx("p",{className:"doc-description",children:"List catalog models."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk catalog list [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--filter-category <string>"})}),e.jsx("td",{children:"Filter catalogs by category name (substring match)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# List all catalog models
kronk catalog list

# List models with local mode (no server required)
kronk catalog list --local

# Filter models by category
kronk catalog list --filter-category embedding`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-pull",children:[e.jsx("h4",{children:"pull"}),e.jsx("p",{className:"doc-description",children:"Pull a model from the catalog."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk catalog pull <MODEL_ID> [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Pull a model from the catalog
kronk catalog pull llama-3.2-1b-q4

# Pull with local mode
kronk catalog pull llama-3.2-1b-q4 --local`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-show",children:[e.jsx("h4",{children:"show"}),e.jsx("p",{className:"doc-description",children:"Show catalog model information."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk catalog show <MODEL_ID> [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Show details for a specific model
kronk catalog show llama-3.2-1b-q4

# Show with local mode
kronk catalog show llama-3.2-1b-q4 --local`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-update",children:[e.jsx("h4",{children:"update"}),e.jsx("p",{className:"doc-description",children:"Update the model catalog."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk catalog update [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Update the catalog from remote source
kronk catalog update

# Update with local mode
kronk catalog update --local`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#usage",className:"doc-index-header",children:"Usage"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#subcommands",className:"doc-index-header",children:"Subcommands"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#cmd-list",children:"list"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-pull",children:"pull"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-show",children:"show"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-update",children:"update"})})]})]})]})})]})]})}function _f(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"libs"}),e.jsx("p",{children:"Install or upgrade llama.cpp libraries."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"usage",children:[e.jsx("h3",{children:"Usage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk libs [flags]"})})]}),e.jsxs("div",{className:"card",id:"subcommands",children:[e.jsx("h3",{children:"Subcommands"}),e.jsxs("div",{className:"doc-section",id:"cmd-(default)",children:[e.jsx("h4",{children:"(default)"}),e.jsx("p",{className:"doc-description",children:"Install or upgrade llama.cpp libraries."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk libs [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_ARCH"})}),e.jsx("td",{children:"runtime.GOARCH"}),e.jsx("td",{children:"The architecture to install (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_LIB_PATH"})}),e.jsx("td",{children:"$HOME/kronk/libraries"}),e.jsx("td",{children:"The path to the libraries directory (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_OS"})}),e.jsx("td",{children:"runtime.GOOS"}),e.jsx("td",{children:"The operating system to install (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_PROCESSOR"})}),e.jsx("td",{children:"cpu"}),e.jsx("td",{children:"Options: cpu, cuda, metal, vulkan (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Install libraries using the server
kronk libs

# Install libraries locally
kronk libs --local

# Install with Metal support on macOS
KRONK_PROCESSOR=metal kronk libs --local`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#usage",className:"doc-index-header",children:"Usage"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#subcommands",className:"doc-index-header",children:"Subcommands"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#cmd-(default)",children:"(default)"})})})]})]})})]})]})}function Pf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"model"}),e.jsx("p",{children:"Manage models - list, pull, remove, show, and check running models."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"usage",children:[e.jsx("h3",{children:"Usage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk model <command> [flags]"})})]}),e.jsxs("div",{className:"card",id:"subcommands",children:[e.jsx("h3",{children:"Subcommands"}),e.jsxs("div",{className:"doc-section",id:"cmd-index",children:[e.jsx("h4",{children:"index"}),e.jsx("p",{className:"doc-description",children:"Rebuild the model index for fast model access."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk model index [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_MODELS"})}),e.jsx("td",{children:"$HOME/kronk/models"}),e.jsx("td",{children:"The path to the models directory (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Rebuild the model index
kronk model index

# Rebuild with local mode
kronk model index --local`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-list",children:[e.jsx("h4",{children:"list"}),e.jsx("p",{className:"doc-description",children:"List models."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk model list [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_MODELS"})}),e.jsx("td",{children:"$HOME/kronk/models"}),e.jsx("td",{children:"The path to the models directory (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# List all models
kronk model list

# List with local mode
kronk model list --local`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-ps",children:[e.jsx("h4",{children:"ps"}),e.jsx("p",{className:"doc-description",children:"List running models."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk model ps"})}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# List running models
kronk model ps`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-pull",children:[e.jsx("h4",{children:"pull"}),e.jsx("p",{className:"doc-description",children:"Pull a model from the web."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk model pull <MODEL_URL> [MMPROJ_URL] [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_MODELS"})}),e.jsx("td",{children:"$HOME/kronk/models"}),e.jsx("td",{children:"The path to the models directory (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Pull a model from a URL
kronk model pull https://huggingface.co/.../model.gguf

# Pull with local mode
kronk model pull https://huggingface.co/.../model.gguf --local

# Pull a vision model with mmproj file
kronk model pull <MODEL_URL> <MMPROJ_URL>`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-remove",children:[e.jsx("h4",{children:"remove"}),e.jsx("p",{className:"doc-description",children:"Remove a model."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk model remove <MODEL_NAME> [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_MODELS"})}),e.jsx("td",{children:"$HOME/kronk/models"}),e.jsx("td",{children:"The path to the models directory (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Remove a model
kronk model remove llama-3.2-1b-q4

# Remove with local mode
kronk model remove llama-3.2-1b-q4 --local`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-show",children:[e.jsx("h4",{children:"show"}),e.jsx("p",{className:"doc-description",children:"Show information for a model."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk model show <MODEL_NAME> [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Authentication token for the kronk server (required when auth enabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_WEB_API_HOST"})}),e.jsx("td",{children:"localhost:8080"}),e.jsx("td",{children:"IP Address for the kronk server (web mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories (local mode)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_MODELS"})}),e.jsx("td",{children:"$HOME/kronk/models"}),e.jsx("td",{children:"The path to the models directory (local mode)"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Show model information
kronk model show llama-3.2-1b-q4

# Show with local mode
kronk model show llama-3.2-1b-q4 --local`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#usage",className:"doc-index-header",children:"Usage"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#subcommands",className:"doc-index-header",children:"Subcommands"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#cmd-index",children:"index"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-list",children:"list"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-ps",children:"ps"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-pull",children:"pull"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-remove",children:"remove"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-show",children:"show"})})]})]})]})})]})]})}function Af(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"run"}),e.jsx("p",{children:"Run an interactive chat session with a model."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"usage",children:[e.jsx("h3",{children:"Usage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk run <MODEL_NAME> [flags]"})})]}),e.jsxs("div",{className:"card",id:"subcommands",children:[e.jsx("h3",{children:"Subcommands"}),e.jsxs("div",{className:"doc-section",id:"cmd-flags",children:[e.jsx("h4",{children:"flags"}),e.jsx("p",{className:"doc-description",children:"Available flags for the run command."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk run <MODEL_NAME> [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--instances <int>"})}),e.jsx("td",{children:"Number of model instances to load (default: 1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--max-tokens <int>"})}),e.jsx("td",{children:"Maximum tokens for response (default: 2048)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--temperature <float>"})}),e.jsx("td",{children:"Temperature for sampling (default: 0.7)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--top-p <float>"})}),e.jsx("td",{children:"Top-p for sampling (default: 0.9)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--top-k <int>"})}),e.jsx("td",{children:"Top-k for sampling (default: 40)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--base-path <string>"})}),e.jsx("td",{children:"Base path for kronk data (models, catalogs, templates)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_MODELS"})}),e.jsx("td",{children:"$HOME/kronk/models"}),e.jsx("td",{children:"The path to the models directory"})]})]})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Start an interactive chat with a model
kronk run Qwen3-8B-Q8_0

# Run with custom sampling parameters
kronk run Qwen3-8B-Q8_0 --temperature 0.5 --top-p 0.95

# Run with higher token limit
kronk run Qwen3-8B-Q8_0 --max-tokens 4096`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#usage",className:"doc-index-header",children:"Usage"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#subcommands",className:"doc-index-header",children:"Subcommands"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#cmd-flags",children:"flags"})})})]})]})})]})]})}function If(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"security"}),e.jsx("p",{children:"Manage security - tokens and access control."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"usage",children:[e.jsx("h3",{children:"Usage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk security <command> [flags]"})})]}),e.jsxs("div",{className:"card",id:"subcommands",children:[e.jsx("h3",{children:"Subcommands"}),e.jsxs("div",{className:"doc-section",id:"cmd-key",children:[e.jsx("h4",{children:"key"}),e.jsx("p",{className:"doc-description",children:"Manage private keys - create and delete private keys."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk security key <command> [flags]"})}),e.jsxs("div",{className:"doc-section",id:"cmd-key-create",children:[e.jsx("h4",{children:"create"}),e.jsx("p",{className:"doc-description",children:"Create a new private key and add it to the keystore."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk security key create [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]})})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Admin token (required when auth enabled)"})]})})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Create a new private key
export KRONK_TOKEN=<admin-token>
kronk security key create`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-key-delete",children:[e.jsx("h4",{children:"delete"}),e.jsx("p",{className:"doc-description",children:"Delete a private key by its key ID."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk security key delete --keyid <KEY_ID> [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--keyid <string>"})}),e.jsx("td",{children:"The key ID to delete (required)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Admin token (required when auth enabled)"})]})})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Delete a private key
export KRONK_TOKEN=<admin-token>
kronk security key delete --keyid abc123`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-key-list",children:[e.jsx("h4",{children:"list"}),e.jsx("p",{className:"doc-description",children:"List all private keys in the system."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk security key list [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]})})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Admin token (required when auth enabled)"})]})})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# List all private keys
export KRONK_TOKEN=<admin-token>
kronk security key list`})})]})]}),e.jsxs("div",{className:"doc-section",id:"cmd-token",children:[e.jsx("h4",{children:"token"}),e.jsx("p",{className:"doc-description",children:"Manage tokens - create and manage security tokens."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk security token <command> [flags]"})}),e.jsxs("div",{className:"doc-section",id:"cmd-token-create",children:[e.jsx("h4",{children:"create"}),e.jsx("p",{className:"doc-description",children:"Create a security token."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk security token create [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--local"})}),e.jsx("td",{children:"Run without the model server"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--duration <duration>"})}),e.jsx("td",{children:"Token duration (e.g., 1h, 24h, 720h)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--endpoints <list>"})}),e.jsx("td",{children:"Endpoints with optional rate limits"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_TOKEN"})}),e.jsx("td",{}),e.jsx("td",{children:"Admin token (required when auth enabled)"})]})})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Create a token with 24 hour duration
export KRONK_TOKEN=<admin-token>
kronk security token create --duration 24h --endpoints chat-completions,embeddings

# Create a token with rate limits
kronk security token create --duration 720h --endpoints "chat-completions:1000/day,embeddings:unlimited"`})})]})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#usage",className:"doc-index-header",children:"Usage"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#subcommands",className:"doc-index-header",children:"Subcommands"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#cmd-key",children:"key"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-key-create",children:"create"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-key-delete",children:"delete"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-key-list",children:"list"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-token",children:"token"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-token-create",children:"create"})})]})]})]})})]})]})}function Of(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"server"}),e.jsx("p",{children:"Manage model server - start, stop, logs."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"usage",children:[e.jsx("h3",{children:"Usage"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk server <command> [flags]"})})]}),e.jsxs("div",{className:"card",id:"subcommands",children:[e.jsx("h3",{children:"Subcommands"}),e.jsxs("div",{className:"doc-section",id:"cmd-start",children:[e.jsx("h4",{children:"start"}),e.jsx("p",{className:"doc-description",children:"Start Kronk model server."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk server start [flags]"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Flag"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"-d, --detach"})}),e.jsx("td",{children:"Run server in the background"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--api-host <string>"})}),e.jsx("td",{children:"API host address (e.g., localhost:8080)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--debug-host <string>"})}),e.jsx("td",{children:"Debug host address (e.g., localhost:8090)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--auth-enabled"})}),e.jsx("td",{children:"Enable local authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--device <string>"})}),e.jsx("td",{children:"Device to use for inference (e.g., cuda, metal)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--max-instances <int>"})}),e.jsx("td",{children:"Maximum model instances"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--models-in-cache <int>"})}),e.jsx("td",{children:"Maximum models in cache"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--cache-ttl <duration>"})}),e.jsx("td",{children:"Cache TTL duration (e.g., 5m, 1h)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--model-config-file <string>"})}),e.jsx("td",{children:"Special config file for model specific config"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"--llama-log <int>"})}),e.jsx("td",{children:"Llama log level (0=off, 1=on)"})]})]})]}),e.jsx("h5",{children:"Environment Variables"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Variable"}),e.jsx("th",{children:"Default"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"KRONK_BASE_PATH"})}),e.jsx("td",{children:"$HOME/kronk"}),e.jsx("td",{children:"Base path for kronk data directories"})]})})]}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Start the server in foreground
kronk server start

# Start the server in background
kronk server start -d

# View all server environment settings
kronk server start --help`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-stop",children:[e.jsx("h4",{children:"stop"}),e.jsx("p",{className:"doc-description",children:"Stop the Kronk model server by sending SIGTERM."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk server stop"})}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Stop the server
kronk server stop`})})]}),e.jsxs("div",{className:"doc-section",id:"cmd-logs",children:[e.jsx("h4",{children:"logs"}),e.jsx("p",{className:"doc-description",children:"Stream the Kronk model server logs (tail -f)."}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"kronk server logs"})}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`# Stream server logs
kronk server logs`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#usage",className:"doc-index-header",children:"Usage"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#subcommands",className:"doc-index-header",children:"Subcommands"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#cmd-start",children:"start"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-stop",children:"stop"})}),e.jsx("li",{children:e.jsx("a",{href:"#cmd-logs",children:"logs"})})]})]})]})})]})]})}function Mf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Chat Completions API"}),e.jsx("p",{children:"Generate chat completions using language models. Compatible with the OpenAI Chat Completions API."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"overview",children:[e.jsx("h3",{children:"Overview"}),e.jsxs("p",{children:["All endpoints are prefixed with ",e.jsx("code",{children:"/v1"}),". Base URL: ",e.jsx("code",{children:"http://localhost:8080"})]}),e.jsx("h4",{children:"Authentication"}),e.jsx("p",{children:"When authentication is enabled, include the token in the Authorization header:"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"Authorization: Bearer YOUR_TOKEN"})})]}),e.jsxs("div",{className:"card",id:"chat-completions",children:[e.jsx("h3",{children:"Chat Completions"}),e.jsx("p",{children:"Create chat completions with language models."}),e.jsxs("div",{className:"doc-section",id:"chat-completions-post--chat-completions",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /chat/completions"]}),e.jsx("p",{className:"doc-description",children:"Create a chat completion. Supports streaming responses."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Token must have 'chat-completions' endpoint access."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Content-Type"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Must be application/json"})]})]})]}),e.jsx("h5",{children:"Request Body"}),e.jsx("p",{children:e.jsx("code",{children:"application/json"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Field"}),e.jsx("th",{children:"Type"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"model"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Model ID to use for completion (e.g., 'qwen3-8b-q8_0')"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"messages"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Array of message objects. See Message Formats section below for supported formats."})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"stream"})}),e.jsx("td",{children:e.jsx("code",{children:"boolean"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Enable streaming responses (default: false)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"tools"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Array of tool definitions for function calling. See Tool Definitions section below."})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"temperature"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Controls randomness of output (default: 0.8)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_k"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Limits token pool to K most probable tokens (default: 40)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_p"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Nucleus sampling threshold (default: 0.9)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"min_p"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Dynamic sampling threshold (default: 0.0)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"max_tokens"})}),e.jsx("td",{children:e.jsx("code",{children:"int"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Maximum output tokens (default: context window)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"repeat_penalty"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Penalty for repeated tokens (default: 1.1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"repeat_last_n"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Recent tokens to consider for repetition penalty (default: 64)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_multiplier"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"DRY sampler multiplier for n-gram repetition penalty (default: 0.0, disabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_base"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Base for exponential penalty growth in DRY (default: 1.75)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_allowed_length"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Minimum n-gram length before DRY applies (default: 2)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_penalty_last_n"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Recent tokens DRY considers, 0 = full context (default: 0)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"xtc_probability"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"XTC probability for extreme token culling (default: 0.0, disabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"xtc_threshold"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Probability threshold for XTC culling (default: 0.1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"xtc_min_keep"})}),e.jsx("td",{children:e.jsx("code",{children:"uint32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Minimum tokens to keep after XTC culling (default: 1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"enable_thinking"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Enable model thinking for non-GPT models (default: true)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"reasoning_effort"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Reasoning level for GPT models: none, minimal, low, medium, high (default: medium)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"return_prompt"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Include prompt in response (default: false)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"include_usage"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Include token usage information in streaming responses (default: true)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"logprobs"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Return log probabilities of output tokens (default: false)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_logprobs"})}),e.jsx("td",{children:e.jsx("code",{children:"int"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Number of most likely tokens to return at each position, 0-5 (default: 0)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"stream"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Stream response as server-sent events (default: false)"})]})]})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a chat completion object, or streams Server-Sent Events if stream=true."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Simple text message:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen3-8b-q8_0",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant."
      },
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Multi-turn conversation:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen3-8b-q8_0",
    "messages": [
      {"role": "user", "content": "What is 2+2?"},
      {"role": "assistant", "content": "2+2 equals 4."},
      {"role": "user", "content": "And what is that multiplied by 3?"}
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Vision - image from URL (requires vision model):"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What is in this image?"},
          {"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
        ]
      }
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Vision - base64 encoded image (requires vision model):"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image"},
          {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQ..."}}
        ]
      }
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Audio - base64 encoded audio (requires audio model):"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen2-audio-7b-q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What is being said in this audio?"},
          {"type": "input_audio", "input_audio": {"data": "UklGRi...", "format": "wav"}}
        ]
      }
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Tool/Function calling - define tools and let the model call them:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen3-8b-q8_0",
    "messages": [
      {"role": "user", "content": "What is the weather in Tokyo?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "The location to get the weather for, e.g. San Francisco, CA"
              }
            },
            "required": ["location"]
          }
        }
      }
    ]
  }'`})})]})]}),e.jsxs("div",{className:"card",id:"response-formats",children:[e.jsx("h3",{children:"Response Formats"}),e.jsx("p",{children:"The response format differs between streaming and non-streaming requests."}),e.jsxs("div",{className:"doc-section",id:"response-formats--non-streaming-response",children:[e.jsx("h4",{children:"Non-Streaming Response"}),e.jsx("p",{className:"doc-description",children:"For non-streaming requests (stream=false or omitted), the response uses the 'message' field in each choice. The 'delta' field is empty."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "qwen3-8b-q8_0",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I'm doing well, thank you for asking.",
        "reasoning": ""
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "reasoning_tokens": 0,
    "completion_tokens": 12,
    "output_tokens": 12,
    "total_tokens": 37,
    "tokens_per_second": 85.5
  }
}`})})]}),e.jsxs("div",{className:"doc-section",id:"response-formats--streaming-response",children:[e.jsx("h4",{children:"Streaming Response"}),e.jsx("p",{className:"doc-description",children:"For streaming requests (stream=true), the response uses the 'delta' field in each choice. Multiple chunks are sent as Server-Sent Events, with incremental content in each delta."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`// Each chunk contains partial content in the delta field
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":" How"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":" are you?"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{...}}
data: [DONE]`})})]})]}),e.jsxs("div",{className:"card",id:"message-formats",children:[e.jsx("h3",{children:"Message Formats"}),e.jsx("p",{children:"The messages array supports several formats depending on the content type and model capabilities."}),e.jsxs("div",{className:"doc-section",id:"message-formats--text-messages",children:[e.jsx("h4",{children:"Text Messages"}),e.jsx("p",{className:"doc-description",children:"Simple text content with role (system, user, or assistant) and content string."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "role": "system",
  "content": "You are a helpful assistant."
}

{
  "role": "user",
  "content": "Hello, how are you?"
}

{
  "role": "assistant",
  "content": "I'm doing well, thank you!"
}`})})]}),e.jsxs("div",{className:"doc-section",id:"message-formats--multi-part-content-(vision)",children:[e.jsx("h4",{children:"Multi-part Content (Vision)"}),e.jsx("p",{className:"doc-description",children:"For vision models, content can be an array with text and image parts. Images can be URLs or base64-encoded data URIs."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "role": "user",
  "content": [
    {"type": "text", "text": "What is in this image?"},
    {"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
  ]
}

// Base64 encoded image
{
  "role": "user",
  "content": [
    {"type": "text", "text": "Describe this image"},
    {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQ..."}}
  ]
}`})})]}),e.jsxs("div",{className:"doc-section",id:"message-formats--audio-content",children:[e.jsx("h4",{children:"Audio Content"}),e.jsx("p",{className:"doc-description",children:"For audio models, content can include audio data as base64-encoded input with format specification."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "role": "user",
  "content": [
    {"type": "text", "text": "What is being said?"},
    {"type": "input_audio", "input_audio": {"data": "UklGRi...", "format": "wav"}}
  ]
}`})})]}),e.jsxs("div",{className:"doc-section",id:"message-formats--tool-definitions",children:[e.jsx("h4",{children:"Tool Definitions"}),e.jsx("p",{className:"doc-description",children:"Tools are defined in the 'tools' array field of the request (not in messages). Each tool specifies a function with name, description, and parameters schema."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`// Tools are defined at the request level
{
  "model": "qwen3-8b-q8_0",
  "messages": [...],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The location to get the weather for, e.g. San Francisco, CA"
            }
          },
          "required": ["location"]
        }
      }
    }
  ]
}`})})]}),e.jsxs("div",{className:"doc-section",id:"message-formats--tool-call-response-(non-streaming)",children:[e.jsx("h4",{children:"Tool Call Response (Non-Streaming)"}),e.jsx("p",{className:"doc-description",children:"For non-streaming requests (stream=false), when the model calls a tool, the response uses the 'message' field with 'tool_calls' array. The finish_reason is 'tool_calls'."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "qwen3-8b-q8_0",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "",
        "tool_calls": [
          {
            "id": "call_xyz789",
            "index": 0,
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\\"location\\":\\"Tokyo\\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ],
  "usage": {
    "prompt_tokens": 50,
    "completion_tokens": 25,
    "total_tokens": 75
  }
}`})})]}),e.jsxs("div",{className:"doc-section",id:"message-formats--tool-call-response-(streaming)",children:[e.jsx("h4",{children:"Tool Call Response (Streaming)"}),e.jsx("p",{className:"doc-description",children:"For streaming requests (stream=true), tool calls are returned in the 'delta' field. Each chunk contains partial tool call data that should be accumulated."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`// Streaming chunks with tool calls use delta instead of message
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"call_xyz789","index":0,"type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\\"location\\":"}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\"Tokyo\\"}"}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{...}}
data: [DONE]`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#overview",className:"doc-index-header",children:"Overview"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#chat-completions",className:"doc-index-header",children:"Chat Completions"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#chat-completions-post--chat-completions",children:"POST /chat/completions"})})})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#response-formats",className:"doc-index-header",children:"Response Formats"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#response-formats--non-streaming-response",children:"Non-Streaming Response"})}),e.jsx("li",{children:e.jsx("a",{href:"#response-formats--streaming-response",children:"Streaming Response"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#message-formats",className:"doc-index-header",children:"Message Formats"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#message-formats--text-messages",children:"Text Messages"})}),e.jsx("li",{children:e.jsx("a",{href:"#message-formats--multi-part-content-(vision)",children:"Multi-part Content (Vision)"})}),e.jsx("li",{children:e.jsx("a",{href:"#message-formats--audio-content",children:"Audio Content"})}),e.jsx("li",{children:e.jsx("a",{href:"#message-formats--tool-definitions",children:"Tool Definitions"})}),e.jsx("li",{children:e.jsx("a",{href:"#message-formats--tool-call-response-(non-streaming)",children:"Tool Call Response (Non-Streaming)"})}),e.jsx("li",{children:e.jsx("a",{href:"#message-formats--tool-call-response-(streaming)",children:"Tool Call Response (Streaming)"})})]})]})]})})]})]})}function Lf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Messages API"}),e.jsx("p",{children:"Generate messages using language models. Compatible with the Anthropic Messages API."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"overview",children:[e.jsx("h3",{children:"Overview"}),e.jsxs("p",{children:["All endpoints are prefixed with ",e.jsx("code",{children:"/v1"}),". Base URL: ",e.jsx("code",{children:"http://localhost:8080"})]}),e.jsx("h4",{children:"Authentication"}),e.jsx("p",{children:"When authentication is enabled, include the token in the Authorization header:"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"Authorization: Bearer YOUR_TOKEN"})})]}),e.jsxs("div",{className:"card",id:"messages",children:[e.jsx("h3",{children:"Messages"}),e.jsx("p",{children:"Create messages with language models using the Anthropic Messages API format."}),e.jsxs("div",{className:"doc-section",id:"messages-post--messages",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /messages"]}),e.jsx("p",{className:"doc-description",children:"Create a message. Supports streaming responses with Server-Sent Events using Anthropic's event format."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Token must have 'messages' endpoint access."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Content-Type"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Must be application/json"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"anthropic-version"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"API version (optional)"})]})]})]}),e.jsx("h5",{children:"Request Body"}),e.jsx("p",{children:e.jsx("code",{children:"application/json"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Field"}),e.jsx("th",{children:"Type"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"model"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"ID of the model to use"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"messages"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Array of message objects with role (user/assistant) and content"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"max_tokens"})}),e.jsx("td",{children:e.jsx("code",{children:"integer"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Maximum number of tokens to generate"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"system"})}),e.jsx("td",{children:e.jsx("code",{children:"string|array"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"System prompt as string or array of content blocks"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"stream"})}),e.jsx("td",{children:e.jsx("code",{children:"boolean"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Enable streaming responses (default: false)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"tools"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"List of tools the model can use"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"temperature"})}),e.jsx("td",{children:e.jsx("code",{children:"number"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Sampling temperature (0-1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_p"})}),e.jsx("td",{children:e.jsx("code",{children:"number"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Nucleus sampling parameter"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_k"})}),e.jsx("td",{children:e.jsx("code",{children:"integer"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Top-k sampling parameter"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"stop_sequences"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Sequences where the API will stop generating"})]})]})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a message object, or streams Server-Sent Events if stream=true. Response includes anthropic-request-id header."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Basic message:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"With system prompt:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "system": "You are a helpful assistant.",
    "messages": [
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Streaming response:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a haiku about coding"}
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Multi-turn conversation:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is 2+2?"},
      {"role": "assistant", "content": "2+2 equals 4."},
      {"role": "user", "content": "What about 2+3?"}
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Vision with image URL (requires vision model):"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What is in this image?"},
          {"type": "image", "source": {"type": "url", "url": "https://example.com/image.jpg"}}
        ]
      }
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Vision with base64 image (requires vision model):"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image"},
          {
            "type": "image",
            "source": {
              "type": "base64",
              "media_type": "image/jpeg",
              "data": "/9j/4AAQ..."
            }
          }
        ]
      }
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Tool calling:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is the weather in Paris?"}
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City name"
            }
          },
          "required": ["location"]
        }
      }
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Tool result (continue conversation after tool call):"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is the weather in Paris?"},
      {
        "role": "assistant",
        "content": [
          {
            "type": "tool_use",
            "id": "call_xyz789",
            "name": "get_weather",
            "input": {"location": "Paris"}
          }
        ]
      },
      {
        "role": "user",
        "content": [
          {
            "type": "tool_result",
            "tool_use_id": "call_xyz789",
            "content": "Sunny, 22°C"
          }
        ]
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {"type": "string"}
          },
          "required": ["location"]
        }
      }
    ]
  }'`})})]})]}),e.jsxs("div",{className:"card",id:"response-formats",children:[e.jsx("h3",{children:"Response Formats"}),e.jsx("p",{children:"The Messages API returns different formats for streaming and non-streaming responses."}),e.jsxs("div",{className:"doc-section",id:"response-formats--non-streaming-response",children:[e.jsx("h4",{children:"Non-Streaming Response"}),e.jsx("p",{className:"doc-description",children:"For non-streaming requests (stream=false or omitted), returns a complete message object."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "Hello! I'm doing well, thank you for asking. How can I help you today?"
    }
  ],
  "model": "qwen3-8b-q8_0",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 12,
    "output_tokens": 18
  }
}`})})]}),e.jsxs("div",{className:"doc-section",id:"response-formats--tool-use-response",children:[e.jsx("h4",{children:"Tool Use Response"}),e.jsx("p",{className:"doc-description",children:"When the model calls a tool, the content includes tool_use blocks with the tool call details."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "tool_use",
      "id": "call_xyz789",
      "name": "get_weather",
      "input": {
        "location": "Paris"
      }
    }
  ],
  "model": "qwen3-8b-q8_0",
  "stop_reason": "tool_use",
  "usage": {
    "input_tokens": 50,
    "output_tokens": 25
  }
}`})})]}),e.jsxs("div",{className:"doc-section",id:"response-formats--streaming-events",children:[e.jsx("h4",{children:"Streaming Events"}),e.jsx("p",{className:"doc-description",children:"For streaming requests (stream=true), the API returns Server-Sent Events with different event types following Anthropic's streaming format."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`event: message_start
data: {"type":"message_start","message":{"id":"msg_abc123","type":"message","role":"assistant","content":[],"model":"qwen3-8b-q8_0","stop_reason":null,"usage":{"input_tokens":12,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":18}}

event: message_stop
data: {"type":"message_stop"}`})})]}),e.jsxs("div",{className:"doc-section",id:"response-formats--streaming-tool-calls",children:[e.jsx("h4",{children:"Streaming Tool Calls"}),e.jsx("p",{className:"doc-description",children:"When streaming tool calls, input_json_delta events provide incremental JSON for tool arguments."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`event: message_start
data: {"type":"message_start","message":{...}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_xyz789","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\\"location\\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\\"Paris\\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":25}}

event: message_stop
data: {"type":"message_stop"}`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#overview",className:"doc-index-header",children:"Overview"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#messages",className:"doc-index-header",children:"Messages"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#messages-post--messages",children:"POST /messages"})})})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#response-formats",className:"doc-index-header",children:"Response Formats"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#response-formats--non-streaming-response",children:"Non-Streaming Response"})}),e.jsx("li",{children:e.jsx("a",{href:"#response-formats--tool-use-response",children:"Tool Use Response"})}),e.jsx("li",{children:e.jsx("a",{href:"#response-formats--streaming-events",children:"Streaming Events"})}),e.jsx("li",{children:e.jsx("a",{href:"#response-formats--streaming-tool-calls",children:"Streaming Tool Calls"})})]})]})]})})]})]})}function Ff(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Responses API"}),e.jsx("p",{children:"Generate responses using language models. Compatible with the OpenAI Responses API."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"overview",children:[e.jsx("h3",{children:"Overview"}),e.jsxs("p",{children:["All endpoints are prefixed with ",e.jsx("code",{children:"/v1"}),". Base URL: ",e.jsx("code",{children:"http://localhost:8080"})]}),e.jsx("h4",{children:"Authentication"}),e.jsx("p",{children:"When authentication is enabled, include the token in the Authorization header:"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"Authorization: Bearer YOUR_TOKEN"})})]}),e.jsxs("div",{className:"card",id:"responses",children:[e.jsx("h3",{children:"Responses"}),e.jsx("p",{children:"Create responses with language models using the Responses API format."}),e.jsxs("div",{className:"doc-section",id:"responses-post--responses",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /responses"]}),e.jsx("p",{className:"doc-description",children:"Create a response. Supports streaming responses with Server-Sent Events."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Token must have 'responses' endpoint access."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Content-Type"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Must be application/json"})]})]})]}),e.jsx("h5",{children:"Request Body"}),e.jsx("p",{children:e.jsx("code",{children:"application/json"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Field"}),e.jsx("th",{children:"Type"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"model"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"ID of the model to use"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"input"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Array of input messages (same format as chat messages)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"stream"})}),e.jsx("td",{children:e.jsx("code",{children:"boolean"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Enable streaming responses (default: false)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"instructions"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"System instructions for the model"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"tools"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"List of tools the model can use"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"tool_choice"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"How the model should use tools: auto, none, or required"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"parallel_tool_calls"})}),e.jsx("td",{children:e.jsx("code",{children:"boolean"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Allow parallel tool calls (default: true)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"store"})}),e.jsx("td",{children:e.jsx("code",{children:"boolean"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Whether to store the response (default: true)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"truncation"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Truncation strategy: auto or disabled (default: disabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"temperature"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Controls randomness of output (default: 0.8)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_k"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Limits token pool to K most probable tokens (default: 40)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_p"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Nucleus sampling threshold (default: 0.9)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"min_p"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Dynamic sampling threshold (default: 0.0)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"max_tokens"})}),e.jsx("td",{children:e.jsx("code",{children:"int"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Maximum output tokens (default: context window)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"repeat_penalty"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Penalty for repeated tokens (default: 1.1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"repeat_last_n"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Recent tokens to consider for repetition penalty (default: 64)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_multiplier"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"DRY sampler multiplier for n-gram repetition penalty (default: 0.0, disabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_base"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Base for exponential penalty growth in DRY (default: 1.75)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_allowed_length"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Minimum n-gram length before DRY applies (default: 2)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dry_penalty_last_n"})}),e.jsx("td",{children:e.jsx("code",{children:"int32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Recent tokens DRY considers, 0 = full context (default: 0)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"xtc_probability"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"XTC probability for extreme token culling (default: 0.0, disabled)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"xtc_threshold"})}),e.jsx("td",{children:e.jsx("code",{children:"float32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Probability threshold for XTC culling (default: 0.1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"xtc_min_keep"})}),e.jsx("td",{children:e.jsx("code",{children:"uint32"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Minimum tokens to keep after XTC culling (default: 1)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"enable_thinking"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Enable model thinking for non-GPT models (default: true)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"reasoning_effort"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Reasoning level for GPT models: none, minimal, low, medium, high (default: medium)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"return_prompt"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Include prompt in response (default: false)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"include_usage"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Include token usage information in streaming responses (default: true)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"logprobs"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Return log probabilities of output tokens (default: false)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_logprobs"})}),e.jsx("td",{children:e.jsx("code",{children:"int"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Number of most likely tokens to return at each position, 0-5 (default: 0)"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"stream"})}),e.jsx("td",{children:e.jsx("code",{children:"bool"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Stream response as server-sent events (default: false)"})]})]})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a response object, or streams Server-Sent Events if stream=true."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Basic response:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/responses \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Streaming response:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/responses \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "Write a short poem about coding"}
    ],
    "stream": true
  }'`})}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"With tools:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/responses \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "What is the weather in London?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string", "description": "City name"}
            },
            "required": ["location"]
          }
        }
      }
    ]
  }'`})})]})]}),e.jsxs("div",{className:"card",id:"response-format",children:[e.jsx("h3",{children:"Response Format"}),e.jsx("p",{children:"The Responses API returns a structured response object with output items."}),e.jsxs("div",{className:"doc-section",id:"response-format--response-object",children:[e.jsx("h4",{children:"Response Object"}),e.jsx("p",{className:"doc-description",children:"The response object contains metadata, output items, and usage information."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "id": "resp_abc123",
  "object": "response",
  "created_at": 1234567890,
  "status": "completed",
  "model": "qwen3-8b-q8_0",
  "output": [
    {
      "type": "message",
      "id": "msg_xyz789",
      "status": "completed",
      "role": "assistant",
      "content": [
        {
          "type": "output_text",
          "text": "Hello! I'm doing well, thank you for asking.",
          "annotations": []
        }
      ]
    }
  ],
  "usage": {
    "input_tokens": 12,
    "output_tokens": 15,
    "total_tokens": 27
  }
}`})})]}),e.jsxs("div",{className:"doc-section",id:"response-format--streaming-events",children:[e.jsx("h4",{children:"Streaming Events"}),e.jsx("p",{className:"doc-description",children:"When stream=true, the API returns Server-Sent Events with different event types."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`event: response.created
data: {"type":"response.created","response":{...}}

event: response.in_progress
data: {"type":"response.in_progress","response":{...}}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{...}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"Hello"}

event: response.output_text.done
data: {"type":"response.output_text.done","text":"Hello! How are you?"}

event: response.completed
data: {"type":"response.completed","response":{...}}`})})]}),e.jsxs("div",{className:"doc-section",id:"response-format--function-call-output",children:[e.jsx("h4",{children:"Function Call Output"}),e.jsx("p",{className:"doc-description",children:"When the model calls a tool, the output contains a function_call item instead of a message."}),e.jsx("h5",{children:"Example"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`{
  "output": [
    {
      "type": "function_call",
      "id": "fc_abc123",
      "call_id": "call_xyz789",
      "name": "get_weather",
      "arguments": "{\\"location\\":\\"London\\"}",
      "status": "completed"
    }
  ]
}`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#overview",className:"doc-index-header",children:"Overview"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#responses",className:"doc-index-header",children:"Responses"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#responses-post--responses",children:"POST /responses"})})})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#response-format",className:"doc-index-header",children:"Response Format"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#response-format--response-object",children:"Response Object"})}),e.jsx("li",{children:e.jsx("a",{href:"#response-format--streaming-events",children:"Streaming Events"})}),e.jsx("li",{children:e.jsx("a",{href:"#response-format--function-call-output",children:"Function Call Output"})})]})]})]})})]})]})}function Df(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Embeddings API"}),e.jsx("p",{children:"Generate vector embeddings for text. Compatible with the OpenAI Embeddings API."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"overview",children:[e.jsx("h3",{children:"Overview"}),e.jsxs("p",{children:["All endpoints are prefixed with ",e.jsx("code",{children:"/v1"}),". Base URL: ",e.jsx("code",{children:"http://localhost:8080"})]}),e.jsx("h4",{children:"Authentication"}),e.jsx("p",{children:"When authentication is enabled, include the token in the Authorization header:"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"Authorization: Bearer YOUR_TOKEN"})})]}),e.jsxs("div",{className:"card",id:"embeddings",children:[e.jsx("h3",{children:"Embeddings"}),e.jsx("p",{children:"Create vector embeddings for semantic search and similarity."}),e.jsxs("div",{className:"doc-section",id:"embeddings-post--embeddings",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /embeddings"]}),e.jsx("p",{className:"doc-description",children:"Create embeddings for the given input text. The model must support embedding generation."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Token must have 'embeddings' endpoint access."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Content-Type"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Must be application/json"})]})]})]}),e.jsx("h5",{children:"Request Body"}),e.jsx("p",{children:e.jsx("code",{children:"application/json"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Field"}),e.jsx("th",{children:"Type"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"model"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Embedding model ID (e.g., 'embeddinggemma-300m-qat-Q8_0')"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"input"})}),e.jsx("td",{children:e.jsx("code",{children:"string|array"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Text to generate embeddings for. Can be a string or array of strings."})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"dimensions"})}),e.jsx("td",{children:e.jsx("code",{children:"integer"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Reduce output to first N dimensions (for Matryoshka models). Must be <= model's native dimensions."})]})]})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns an embedding object with vector data."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Generate embeddings for text:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/embeddings \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "embeddinggemma-300m-qat-Q8_0",
    "input": "Why is the sky blue?"
  }'`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#overview",className:"doc-index-header",children:"Overview"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#embeddings",className:"doc-index-header",children:"Embeddings"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#embeddings-post--embeddings",children:"POST /embeddings"})})})]})]})})]})]})}function Uf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Rerank API"}),e.jsx("p",{children:"Rerank documents by relevance to a query. Used for semantic search result ordering."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"overview",children:[e.jsx("h3",{children:"Overview"}),e.jsxs("p",{children:["All endpoints are prefixed with ",e.jsx("code",{children:"/v1"}),". Base URL: ",e.jsx("code",{children:"http://localhost:8080"})]}),e.jsx("h4",{children:"Authentication"}),e.jsx("p",{children:"When authentication is enabled, include the token in the Authorization header:"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"Authorization: Bearer YOUR_TOKEN"})})]}),e.jsxs("div",{className:"card",id:"reranking",children:[e.jsx("h3",{children:"Reranking"}),e.jsx("p",{children:"Score and reorder documents by relevance to a query."}),e.jsxs("div",{className:"doc-section",id:"reranking-post--rerank",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /rerank"]}),e.jsx("p",{className:"doc-description",children:"Rerank documents by their relevance to a query. The model must support reranking."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Token must have 'rerank' endpoint access."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Content-Type"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Must be application/json"})]})]})]}),e.jsx("h5",{children:"Request Body"}),e.jsx("p",{children:e.jsx("code",{children:"application/json"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Field"}),e.jsx("th",{children:"Type"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"model"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Reranker model ID (e.g., 'bge-reranker-v2-m3-Q8_0')"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"query"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"The query to rank documents against."})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"documents"})}),e.jsx("td",{children:e.jsx("code",{children:"array"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Array of document strings to rank."})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"top_n"})}),e.jsx("td",{children:e.jsx("code",{children:"integer"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Return only the top N results. Defaults to all documents."})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"return_documents"})}),e.jsx("td",{children:e.jsx("code",{children:"boolean"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Include document text in results. Defaults to false."})]})]})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a list of reranked results with index and relevance_score, sorted by score descending."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Rerank documents for a query:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/rerank \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "bge-reranker-v2-m3-Q8_0",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of artificial intelligence.",
      "The weather today is sunny.",
      "Deep learning uses neural networks."
    ],
    "top_n": 2
  }'`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#overview",className:"doc-index-header",children:"Overview"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#reranking",className:"doc-index-header",children:"Reranking"}),e.jsx("ul",{children:e.jsx("li",{children:e.jsx("a",{href:"#reranking-post--rerank",children:"POST /rerank"})})})]})]})})]})]})}function Kf(){return e.jsxs("div",{children:[e.jsxs("div",{className:"page-header",children:[e.jsx("h2",{children:"Tools API"}),e.jsx("p",{children:"Manage libraries, models, catalog, and security. These endpoints handle server administration tasks."})]}),e.jsxs("div",{className:"doc-layout",children:[e.jsxs("div",{className:"doc-content",children:[e.jsxs("div",{className:"card",id:"overview",children:[e.jsx("h3",{children:"Overview"}),e.jsxs("p",{children:["All endpoints are prefixed with ",e.jsx("code",{children:"/v1"}),". Base URL: ",e.jsx("code",{children:"http://localhost:8080"})]}),e.jsx("h4",{children:"Authentication"}),e.jsx("p",{children:"When authentication is enabled, include the token in the Authorization header:"}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"Authorization: Bearer YOUR_TOKEN"})})]}),e.jsxs("div",{className:"card",id:"libs",children:[e.jsx("h3",{children:"Libs"}),e.jsx("p",{children:"Manage llama.cpp libraries installation and updates."}),e.jsxs("div",{className:"doc-section",id:"libs-get--libs",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /libs"]}),e.jsx("p",{className:"doc-description",children:"Get information about installed llama.cpp libraries."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns version information including arch, os, processor, latest version, and current version."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Get library information:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X GET http://localhost:8080/v1/libs"})})]}),e.jsxs("div",{className:"doc-section",id:"libs-post--libs-pull",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /libs/pull"]}),e.jsx("p",{className:"doc-description",children:"Download and install the latest llama.cpp libraries. Returns streaming progress updates."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Streams download progress as Server-Sent Events."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Pull latest libraries:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X POST http://localhost:8080/v1/libs/pull"})})]})]}),e.jsxs("div",{className:"card",id:"models",children:[e.jsx("h3",{children:"Models"}),e.jsx("p",{children:"Manage models - list, pull, show, and remove models from the server."}),e.jsxs("div",{className:"doc-section",id:"models-get--models",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /models"]}),e.jsx("p",{className:"doc-description",children:"List all available models on the server."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a list of model objects with id, owned_by, model_family, size, and modified fields."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"List all models:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X GET http://localhost:8080/v1/models"})})]}),e.jsxs("div",{className:"doc-section",id:"models-get--models-model",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /models/{model}"]}),e.jsx("p",{className:"doc-description",children:"Show detailed information about a specific model."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns model details including metadata, capabilities, and configuration."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Show model details:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X GET http://localhost:8080/v1/models/qwen3-8b-q8_0"})})]}),e.jsxs("div",{className:"doc-section",id:"models-get--models-ps",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /models/ps"]}),e.jsx("p",{className:"doc-description",children:"List currently loaded/running models in the cache."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a list of running models with id, owned_by, model_family, size, expires_at, and active_streams."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"List running models:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X GET http://localhost:8080/v1/models/ps"})})]}),e.jsxs("div",{className:"doc-section",id:"models-post--models-index",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /models/index"]}),e.jsx("p",{className:"doc-description",children:"Rebuild the model index for fast model access."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns empty response on success."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Rebuild model index:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X POST http://localhost:8080/v1/models/index"})})]}),e.jsxs("div",{className:"doc-section",id:"models-post--models-pull",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /models/pull"]}),e.jsx("p",{className:"doc-description",children:"Pull/download a model from a URL. Returns streaming progress updates."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Content-Type"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Must be application/json"})]})]})]}),e.jsx("h5",{children:"Request Body"}),e.jsx("p",{children:e.jsx("code",{children:"application/json"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Field"}),e.jsx("th",{children:"Type"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"model_url"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"URL to the model GGUF file"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"proj_url"})}),e.jsx("td",{children:e.jsx("code",{children:"string"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"URL to the projection file (for vision/audio models)"})]})]})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Streams download progress as Server-Sent Events."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Pull a model from HuggingFace:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/models/pull \\
  -H "Content-Type: application/json" \\
  -d '{
    "model_url": "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
  }'`})})]}),e.jsxs("div",{className:"doc-section",id:"models-delete--models-model",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-delete",children:"DELETE"})," /models/{model}"]}),e.jsx("p",{className:"doc-description",children:"Remove a model from the server."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns empty response on success."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Remove a model:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X DELETE http://localhost:8080/v1/models/qwen3-8b-q8_0"})})]})]}),e.jsxs("div",{className:"card",id:"catalog",children:[e.jsx("h3",{children:"Catalog"}),e.jsx("p",{children:"Browse and pull models from the curated model catalog."}),e.jsxs("div",{className:"doc-section",id:"catalog-get--catalog",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /catalog"]}),e.jsx("p",{className:"doc-description",children:"List all models available in the catalog."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a list of catalog models with id, category, owned_by, model_family, and capabilities."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"List catalog models:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X GET http://localhost:8080/v1/catalog"})})]}),e.jsxs("div",{className:"doc-section",id:"catalog-get--catalog-filter-filter",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /catalog/filter/{filter}"]}),e.jsx("p",{className:"doc-description",children:"List catalog models filtered by category."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a filtered list of catalog models."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Filter catalog by category:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X GET http://localhost:8080/v1/catalog/filter/embedding"})})]}),e.jsxs("div",{className:"doc-section",id:"catalog-get--catalog-model",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /catalog/{model}"]}),e.jsx("p",{className:"doc-description",children:"Show detailed information about a catalog model."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns full catalog model details including files, capabilities, and metadata."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Show catalog model details:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X GET http://localhost:8080/v1/catalog/qwen3-8b-q8_0"})})]}),e.jsxs("div",{className:"doc-section",id:"catalog-post--catalog-pull-model",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /catalog/pull/{model}"]}),e.jsx("p",{className:"doc-description",children:"Pull a model from the catalog by ID. Returns streaming progress updates."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Optional when auth is enabled."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Bearer token for authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Streams download progress as Server-Sent Events."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Pull a catalog model:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:"curl -X POST http://localhost:8080/v1/catalog/pull/qwen3-8b-q8_0"})})]})]}),e.jsxs("div",{className:"card",id:"security",children:[e.jsx("h3",{children:"Security"}),e.jsx("p",{children:"Manage security tokens and private keys for authentication."}),e.jsxs("div",{className:"doc-section",id:"security-post--security-token-create",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /security/token/create"]}),e.jsx("p",{className:"doc-description",children:"Create a new security token with specified permissions and duration."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Content-Type"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Must be application/json"})]})]})]}),e.jsx("h5",{children:"Request Body"}),e.jsx("p",{children:e.jsx("code",{children:"application/json"})}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Field"}),e.jsx("th",{children:"Type"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsxs("tbody",{children:[e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"admin"})}),e.jsx("td",{children:e.jsx("code",{children:"boolean"})}),e.jsx("td",{children:"No"}),e.jsx("td",{children:"Whether the token has admin privileges"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"duration"})}),e.jsx("td",{children:e.jsx("code",{children:"duration"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Token validity duration (e.g., '24h', '720h')"})]}),e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"endpoints"})}),e.jsx("td",{children:e.jsx("code",{children:"object"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Map of endpoint names to rate limit configurations"})]})]})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns the created token string."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Create a token with chat-completions access:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/security/token/create \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "admin": false,
    "duration": "24h",
    "endpoints": {
      "chat-completions": {"limit": 1000, "window": "day"},
      "embeddings": {"limit": 0, "window": ""}
    }
  }'`})})]}),e.jsxs("div",{className:"doc-section",id:"security-get--security-keys",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-get",children:"GET"})," /security/keys"]}),e.jsx("p",{className:"doc-description",children:"List all private keys in the system."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns a list of keys with id and created timestamp."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"List all keys:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X GET http://localhost:8080/v1/security/keys \\
  -H "Authorization: Bearer $KRONK_TOKEN"`})})]}),e.jsxs("div",{className:"doc-section",id:"security-post--security-keys-add",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /security/keys/add"]}),e.jsx("p",{className:"doc-description",children:"Create a new private key and add it to the keystore."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns empty response on success."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Add a new key:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/security/keys/add \\
  -H "Authorization: Bearer $KRONK_TOKEN"`})})]}),e.jsxs("div",{className:"doc-section",id:"security-post--security-keys-remove-keyid",children:[e.jsxs("h4",{children:[e.jsx("span",{className:"method-post",children:"POST"})," /security/keys/remove/{keyid}"]}),e.jsx("p",{className:"doc-description",children:"Remove a private key from the keystore by its ID."}),e.jsxs("p",{children:[e.jsx("strong",{children:"Authentication:"})," Required when auth is enabled. Admin token required."]}),e.jsx("h5",{children:"Headers"}),e.jsxs("table",{className:"flags-table",children:[e.jsx("thead",{children:e.jsxs("tr",{children:[e.jsx("th",{children:"Header"}),e.jsx("th",{children:"Required"}),e.jsx("th",{children:"Description"})]})}),e.jsx("tbody",{children:e.jsxs("tr",{children:[e.jsx("td",{children:e.jsx("code",{children:"Authorization"})}),e.jsx("td",{children:"Yes"}),e.jsx("td",{children:"Bearer token for admin authentication"})]})})]}),e.jsx("h5",{children:"Response"}),e.jsx("p",{children:"Returns empty response on success."}),e.jsx("h5",{children:"Example"}),e.jsx("p",{className:"example-label",children:e.jsx("strong",{children:"Remove a key:"})}),e.jsx("pre",{className:"code-block",children:e.jsx("code",{children:`curl -X POST http://localhost:8080/v1/security/keys/remove/abc123 \\
  -H "Authorization: Bearer $KRONK_TOKEN"`})})]})]})]}),e.jsx("nav",{className:"doc-sidebar",children:e.jsxs("div",{className:"doc-sidebar-content",children:[e.jsx("div",{className:"doc-index-section",children:e.jsx("a",{href:"#overview",className:"doc-index-header",children:"Overview"})}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#libs",className:"doc-index-header",children:"Libs"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#libs-get--libs",children:"GET /libs"})}),e.jsx("li",{children:e.jsx("a",{href:"#libs-post--libs-pull",children:"POST /libs/pull"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#models",className:"doc-index-header",children:"Models"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#models-get--models",children:"GET /models"})}),e.jsx("li",{children:e.jsx("a",{href:"#models-get--models-model",children:"GET /models/{model}"})}),e.jsx("li",{children:e.jsx("a",{href:"#models-get--models-ps",children:"GET /models/ps"})}),e.jsx("li",{children:e.jsx("a",{href:"#models-post--models-index",children:"POST /models/index"})}),e.jsx("li",{children:e.jsx("a",{href:"#models-post--models-pull",children:"POST /models/pull"})}),e.jsx("li",{children:e.jsx("a",{href:"#models-delete--models-model",children:"DELETE /models/{model}"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#catalog",className:"doc-index-header",children:"Catalog"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#catalog-get--catalog",children:"GET /catalog"})}),e.jsx("li",{children:e.jsx("a",{href:"#catalog-get--catalog-filter-filter",children:"GET /catalog/filter/{filter}"})}),e.jsx("li",{children:e.jsx("a",{href:"#catalog-get--catalog-model",children:"GET /catalog/{model}"})}),e.jsx("li",{children:e.jsx("a",{href:"#catalog-post--catalog-pull-model",children:"POST /catalog/pull/{model}"})})]})]}),e.jsxs("div",{className:"doc-index-section",children:[e.jsx("a",{href:"#security",className:"doc-index-header",children:"Security"}),e.jsxs("ul",{children:[e.jsx("li",{children:e.jsx("a",{href:"#security-post--security-token-create",children:"POST /security/token/create"})}),e.jsx("li",{children:e.jsx("a",{href:"#security-get--security-keys",children:"GET /security/keys"})}),e.jsx("li",{children:e.jsx("a",{href:"#security-post--security-keys-add",children:"POST /security/keys/add"})}),e.jsx("li",{children:e.jsx("a",{href:"#security-post--security-keys-remove-keyid",children:"POST /security/keys/remove/{keyid}"})})]})]})]})})]})]})}const pi={home:"/",chat:"/chat","model-list":"/models","model-ps":"/models/running","model-pull":"/models/pull","catalog-list":"/catalog","libs-pull":"/libs/pull","security-key-list":"/security/keys","security-key-create":"/security/keys/create","security-key-delete":"/security/keys/delete","security-token-create":"/security/tokens/create",settings:"/settings","docs-sdk":"/docs/sdk","docs-sdk-kronk":"/docs/sdk/kronk","docs-sdk-model":"/docs/sdk/model","docs-sdk-examples":"/docs/sdk/examples","docs-cli-catalog":"/docs/cli/catalog","docs-cli-libs":"/docs/cli/libs","docs-cli-model":"/docs/cli/model","docs-cli-run":"/docs/cli/run","docs-cli-security":"/docs/cli/security","docs-cli-server":"/docs/cli/server","docs-api-chat":"/docs/api/chat","docs-api-messages":"/docs/api/messages","docs-api-responses":"/docs/api/responses","docs-api-embeddings":"/docs/api/embeddings","docs-api-rerank":"/docs/api/rerank","docs-api-tools":"/docs/api/tools"},Bf=Object.fromEntries(Object.entries(pi).map(([n,t])=>[t,n]));function zf(){return e.jsxs("div",{className:"home-page",children:[e.jsxs("div",{className:"hero-section",children:[e.jsx("img",{src:"https://raw.githubusercontent.com/ardanlabs/kronk/refs/heads/main/images/project/kronk_banner.jpg",alt:"Kronk Banner",className:"hero-banner"}),e.jsx("p",{className:"hero-tagline",children:"Hardware-accelerated local inference with llama.cpp directly integrated into your Go applications"})]}),e.jsxs("div",{className:"features-grid",children:[e.jsxs("div",{className:"feature-card",children:[e.jsx("div",{className:"feature-icon",children:"🚀"}),e.jsx("h3",{children:"High-Level Go API"}),e.jsx("p",{children:"Feels similar to using an OpenAI compatible API, but runs entirely on your hardware"})]}),e.jsxs("div",{className:"feature-card",children:[e.jsx("div",{className:"feature-icon",children:"🔧"}),e.jsx("h3",{children:"OpenAI Compatible Server"}),e.jsx("p",{children:"Model server for chat completions and embeddings, compatible with OpenWebUI"})]}),e.jsxs("div",{className:"feature-card",children:[e.jsx("div",{className:"feature-icon",children:"🎯"}),e.jsx("h3",{children:"Multimodal Support"}),e.jsx("p",{children:"Text, vision, and audio models with full hardware acceleration"})]}),e.jsxs("div",{className:"feature-card",children:[e.jsx("div",{className:"feature-icon",children:"⚡"}),e.jsx("h3",{children:"GPU Acceleration"}),e.jsx("p",{children:"Metal on macOS, CUDA/Vulkan/ROCm on Linux, CUDA/Vulkan on Windows"})]})]}),e.jsx("div",{className:"home-cta",children:e.jsx("p",{children:"Use the sidebar to manage models, browse the catalog, or explore the SDK documentation."})})]})}function $f(){return e.jsx(zp,{children:e.jsx(lf,{children:e.jsx(Wp,{children:e.jsx(Vp,{children:e.jsx(Qp,{children:e.jsxs(Mp,{children:[e.jsx($,{path:"/",element:e.jsx(zf,{})}),e.jsx($,{path:"/chat",element:e.jsx(jf,{})}),e.jsx($,{path:"/models",element:e.jsx(Zp,{})}),e.jsx($,{path:"/models/running",element:e.jsx(nf,{})}),e.jsx($,{path:"/models/pull",element:e.jsx(tf,{})}),e.jsx($,{path:"/catalog",element:e.jsx(rf,{})}),e.jsx($,{path:"/libs/pull",element:e.jsx(sf,{})}),e.jsx($,{path:"/security/keys",element:e.jsx(of,{})}),e.jsx($,{path:"/security/keys/create",element:e.jsx(af,{})}),e.jsx($,{path:"/security/keys/delete",element:e.jsx(cf,{})}),e.jsx($,{path:"/security/tokens/create",element:e.jsx(hf,{})}),e.jsx($,{path:"/settings",element:e.jsx(mf,{})}),e.jsx($,{path:"/docs/sdk",element:e.jsx(gf,{})}),e.jsx($,{path:"/docs/sdk/kronk",element:e.jsx(vf,{})}),e.jsx($,{path:"/docs/sdk/model",element:e.jsx(yf,{})}),e.jsx($,{path:"/docs/sdk/examples",element:e.jsx(Rf,{})}),e.jsx($,{path:"/docs/cli/catalog",element:e.jsx(Cf,{})}),e.jsx($,{path:"/docs/cli/libs",element:e.jsx(_f,{})}),e.jsx($,{path:"/docs/cli/model",element:e.jsx(Pf,{})}),e.jsx($,{path:"/docs/cli/run",element:e.jsx(Af,{})}),e.jsx($,{path:"/docs/cli/security",element:e.jsx(If,{})}),e.jsx($,{path:"/docs/cli/server",element:e.jsx(Of,{})}),e.jsx($,{path:"/docs/api/chat",element:e.jsx(Mf,{})}),e.jsx($,{path:"/docs/api/messages",element:e.jsx(Lf,{})}),e.jsx($,{path:"/docs/api/responses",element:e.jsx(Ff,{})}),e.jsx($,{path:"/docs/api/embeddings",element:e.jsx(Df,{})}),e.jsx($,{path:"/docs/api/rerank",element:e.jsx(Uf,{})}),e.jsx($,{path:"/docs/api/tools",element:e.jsx(Kf,{})})]})})})})})})}gl.createRoot(document.getElementById("root")).render(e.jsx(Wa.StrictMode,{children:e.jsx($f,{})}));
