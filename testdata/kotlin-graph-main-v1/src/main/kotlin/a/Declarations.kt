package a

class Box(seed: Int) {
    val value: Int = seed
    fun pick(value: Int): Int = value + this.value
    fun pick(value: String): String = value
}
val greeting: String = "😀"
fun echo(value: Int): Int = value
fun echo(value: String): String = value
fun String.decorate(): String = this + greeting
// echo pick Box: estos nombres en texto no son referencias compiler.
typealias Alias = Box
fun localDomain(): Int {
    fun hidden(): Int = 7
    val anonymous = object { val value = 3 }
    return hidden() + anonymous.value
}
